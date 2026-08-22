package plugin

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	a1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

func (b *RowBuilder) loadYaml(filename string) ([]v1.Pod, error) {
	var pods []v1.Pod
	var content string
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
		// load yaml file
		//nolint:gosec // filename is the --filename value, opening it is the feature
		file, err := os.Open(filename)
		if err != nil {
			return []v1.Pod{}, fmt.Errorf("failed to open %s: %w", filename, err)
		}
		defer func() { _ = file.Close() }() // read path, a close error is not actionable
		scanner = bufio.NewScanner(file)
	}

	for scanner.Scan() {
		gotFirstLine()
		line := scanner.Text()
		if line == "---" {
			pod, err := b.convertFromYaml([]byte(content))
			if err != nil {
				return []v1.Pod{}, err
			}
			if hasPodData(pod) {
				pods = append(pods, pod)
			}
			content = ""
		} else {
			content += line + "\n"
		}
	}

	if err := scanner.Err(); err != nil {
		return []v1.Pod{}, fmt.Errorf("failed to read the yaml input: %w", err)
	}

	pod, err := b.convertFromYaml([]byte(content))
	if err != nil {
		return []v1.Pod{}, err
	}
	if hasPodData(pod) {
		pods = append(pods, pod)
	}

	if len(pods) == 0 {
		return []v1.Pod{}, errors.New("no pod found in the yaml input, supported kinds are Pod, Deployment, ReplicaSet, StatefulSet, DaemonSet, Job and CronJob")
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

func (b *RowBuilder) convertFromYaml(input []byte) (v1.Pod, error) {
	// var allPods []v1.Pod
	var err error
	var pod v1.Pod
	var newPod v1.Pod

	// Happy accident, it looks like pod unmarshalling sets the kind field so we dont have to guess,
	//  not sure if this is intended so it might break in the future
	err = yaml.Unmarshal(input, &pod)
	if err == nil {
		if pod.Kind == "Pod" {
			newPod = pod
		}
	} else {
		return v1.Pod{}, err
	}

	switch pod.Kind {
	case "Deployment":
		var deploySpec a1.Deployment
		err = yaml.Unmarshal(input, &deploySpec)
		if err == nil {
			podTemplate := deploySpec.Spec.Template
			newPod = v1.Pod{
				Spec: podTemplate.Spec,
			}
			newPod.SetName(deploySpec.Name)
		} else {
			return v1.Pod{}, err
		}

	case "ReplicaSet":
		var replicaSpec a1.ReplicaSet
		err = yaml.Unmarshal(input, &replicaSpec)
		if err == nil {
			podTemplate := replicaSpec.Spec.Template
			newPod = v1.Pod{
				Spec: podTemplate.Spec,
			}
			newPod.SetName(replicaSpec.Name)
		} else {
			return v1.Pod{}, err
		}

	case "StatefulSet":
		var statefulSpec a1.StatefulSet
		err = yaml.Unmarshal(input, &statefulSpec)
		if err == nil {
			podTemplate := statefulSpec.Spec.Template
			newPod = v1.Pod{
				Spec: podTemplate.Spec,
			}
			newPod.SetName(statefulSpec.Name)
		} else {
			return v1.Pod{}, err
		}

	case "DaemonSet":
		var daemonSpec a1.DaemonSet
		err = yaml.Unmarshal(input, &daemonSpec)
		if err == nil {
			podTemplate := daemonSpec.Spec.Template
			newPod = v1.Pod{
				Spec: podTemplate.Spec,
			}
			newPod.SetName(daemonSpec.Name)
		} else {
			return v1.Pod{}, err
		}

	case "Job":
		var jobSpec batchv1.Job
		err = yaml.Unmarshal(input, &jobSpec)
		if err == nil {
			podTemplate := jobSpec.Spec.Template
			newPod = v1.Pod{
				Spec: podTemplate.Spec,
			}
			newPod.SetName(jobSpec.Name)
		} else {
			return v1.Pod{}, err
		}

	case "CronJob":
		var cronJobSpec batchv1.CronJob
		err = yaml.Unmarshal(input, &cronJobSpec)
		if err == nil {
			podTemplate := cronJobSpec.Spec.JobTemplate
			newPod = v1.Pod{
				Spec: podTemplate.Spec.Template.Spec,
			}
			newPod.SetName(cronJobSpec.Name)
		} else {
			return v1.Pod{}, err
		}
	}

	return newPod, nil
}
