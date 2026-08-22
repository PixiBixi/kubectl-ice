package plugin

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	a1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

func (b *RowBuilder) loadYaml(filename string) ([]v1.Pod, error) {
	var pods []v1.Pod
	var scanner *bufio.Scanner

	// noop unless we read stdin, see the goroutine below
	gotFirstLine := func() {}

	if b.StdinChanged {
		// a pipe with no data blocks here forever, and a silent hang reads as a
		// stuck api call. Say what we wait for, but only while we really wait:
		// the timer is cancelled by the first line, not by the end of the parse.
		firstLine := make(chan struct{})
		gotFirstLine = sync.OnceFunc(func() { close(firstLine) })
		defer gotFirstLine()

		go func() {
			select {
			case <-firstLine:
			case <-time.After(stdinWaitNotice):
				fmt.Fprintln(os.Stderr, "waiting for yaml on stdin, press ctrl-c to cancel")
			}
		}()

		// read yaml from stdin
		file := bufio.NewReader(os.Stdin)
		scanner = bufio.NewScanner(file)
	} else {
		//nolint:gosec // filename is the --filename value, opening it is the feature
		file, err := os.Open(filename)
		if err != nil {
			return []v1.Pod{}, fmt.Errorf("failed to open %s: %w", filename, err)
		}
		defer func() { _ = file.Close() }() // read path, a close error is not actionable

		scanner = bufio.NewScanner(file)
	}

	// a strings.Builder rather than growing a string by reassignment: one
	// `kubectl get pods -o yaml` is a single document tens of thousands of lines
	// long, and the old buffer made that quadratic.
	var content strings.Builder

	addDocument := func() error {
		if content.Len() == 0 {
			return nil
		}

		found, err := b.convertFromYaml([]byte(content.String()))
		if err != nil {
			return err
		}
		pods = append(pods, found...)
		content.Reset()

		return nil
	}

	for scanner.Scan() {
		gotFirstLine()

		if line := scanner.Text(); line == "---" {
			if err := addDocument(); err != nil {
				return []v1.Pod{}, err
			}
		} else {
			content.WriteString(line)
			content.WriteString("\n")
		}
	}

	if err := scanner.Err(); err != nil {
		return []v1.Pod{}, fmt.Errorf("failed to read the yaml input: %w", err)
	}

	if err := addDocument(); err != nil {
		return []v1.Pod{}, err
	}

	if len(pods) == 0 {
		return []v1.Pod{}, errors.New("no pod found in the yaml input, supported kinds are List, Pod, Deployment, ReplicaSet, StatefulSet, DaemonSet, Job and CronJob")
	}

	return pods, nil
}

// stdinWaitNotice is how long we read stdin before telling the user we are
// waiting on them. Short enough to catch an accidental pipe, long enough that a
// normal `kubectl get -o yaml | ice` never prints anything.
const stdinWaitNotice = 2 * time.Second

// hasPodData reports whether convertFromYaml actually recognised the document.
// An unsupported kind yields a zero Pod, which used to be appended anyway and
// rendered as an empty table with no hint that the input was not understood.
func hasPodData(pod v1.Pod) bool {
	return pod.Name != "" || len(pod.Spec.Containers) > 0 || len(pod.Spec.InitContainers) > 0
}

// convertFromYaml turns one yaml document into the pods it describes. A workload
// contributes its pod template, a List contributes each of its items, and an
// unrecognised kind contributes nothing.
func (b *RowBuilder) convertFromYaml(input []byte) ([]v1.Pod, error) {
	var pod v1.Pod

	// pod unmarshalling fills the kind field, so the dispatch below does not need
	// a separate probe of the document
	if err := yaml.Unmarshal(input, &pod); err != nil {
		return nil, err
	}

	// fromTemplate builds a pod out of a workload's template, keeping the
	// workload's name so the output identifies what it came from
	fromTemplate := func(name string, template v1.PodTemplateSpec) []v1.Pod {
		out := v1.Pod{Spec: template.Spec}
		out.SetName(name)
		if !hasPodData(out) {
			return nil
		}

		return []v1.Pod{out}
	}

	switch pod.Kind {
	case "Pod":
		if !hasPodData(pod) {
			return nil, nil
		}

		return []v1.Pod{pod}, nil

	case "List":
		// what `kubectl get pods -o yaml` produces: one document wrapping every
		// item, rather than one document per pod
		var list v1.List
		if err := yaml.Unmarshal(input, &list); err != nil {
			return nil, err
		}

		var pods []v1.Pod
		for i := range list.Items {
			found, err := b.convertFromYaml(list.Items[i].Raw)
			if err != nil {
				return nil, err
			}
			pods = append(pods, found...)
		}

		return pods, nil

	case "Deployment":
		var spec a1.Deployment
		if err := yaml.Unmarshal(input, &spec); err != nil {
			return nil, err
		}

		return fromTemplate(spec.Name, spec.Spec.Template), nil

	case "ReplicaSet":
		var spec a1.ReplicaSet
		if err := yaml.Unmarshal(input, &spec); err != nil {
			return nil, err
		}

		return fromTemplate(spec.Name, spec.Spec.Template), nil

	case "StatefulSet":
		var spec a1.StatefulSet
		if err := yaml.Unmarshal(input, &spec); err != nil {
			return nil, err
		}

		return fromTemplate(spec.Name, spec.Spec.Template), nil

	case "DaemonSet":
		var spec a1.DaemonSet
		if err := yaml.Unmarshal(input, &spec); err != nil {
			return nil, err
		}

		return fromTemplate(spec.Name, spec.Spec.Template), nil

	case "Job":
		var spec batchv1.Job
		if err := yaml.Unmarshal(input, &spec); err != nil {
			return nil, err
		}

		return fromTemplate(spec.Name, spec.Spec.Template), nil

	case "CronJob":
		var spec batchv1.CronJob
		if err := yaml.Unmarshal(input, &spec); err != nil {
			return nil, err
		}

		return fromTemplate(spec.Name, spec.Spec.JobTemplate.Spec.Template), nil
	}

	return nil, nil
}
