package plugin

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// looperTestPod is a container that exercises as many columns as possible: every
// probe kind, resources, ports, env, volume mounts, a security context and
// lifecycle hooks.
func looperTestPod() v1.Pod {
	probe := func(port int32) *v1.Probe {
		return &v1.Probe{
			HTTPGet:             &v1.HTTPGetAction{Path: "/healthz", Port: intstr.FromInt32(port)},
			InitialDelaySeconds: 5, PeriodSeconds: 10, TimeoutSeconds: 1,
			SuccessThreshold: 1, FailureThreshold: 3,
		}
	}

	container := v1.Container{
		Name:            "app",
		Image:           "eu.gcr.io/project/app:v1.2.3",
		ImagePullPolicy: v1.PullIfNotPresent,
		Command:         []string{"/bin/app"},
		Args:            []string{"--flag", "value"},
		Ports:           []v1.ContainerPort{{Name: "http", ContainerPort: 8080, HostPort: 18080, Protocol: v1.ProtocolTCP}},
		Env:             []v1.EnvVar{{Name: "LOG_LEVEL", Value: "info"}},
		Resources: v1.ResourceRequirements{
			Requests: v1.ResourceList{"cpu": apiresource.MustParse("100m"), "memory": apiresource.MustParse("256Mi")},
			Limits:   v1.ResourceList{"cpu": apiresource.MustParse("500m"), "memory": apiresource.MustParse("512Mi")},
		},
		VolumeMounts:   []v1.VolumeMount{{Name: "data", MountPath: "/var/data", ReadOnly: true}},
		LivenessProbe:  probe(8080),
		ReadinessProbe: probe(8081),
		StartupProbe:   probe(8082),
		Lifecycle: &v1.Lifecycle{
			PostStart: &v1.LifecycleHandler{Exec: &v1.ExecAction{Command: []string{"/bin/warmup"}}},
			PreStop:   &v1.LifecycleHandler{Exec: &v1.ExecAction{Command: []string{"/bin/drain"}}},
		},
		SecurityContext: &v1.SecurityContext{
			RunAsUser:              new(int64(1000)),
			RunAsGroup:             new(int64(2000)),
			RunAsNonRoot:           new(true),
			Privileged:             new(false),
			ReadOnlyRootFilesystem: new(true),
			Capabilities:           &v1.Capabilities{Add: []v1.Capability{"NET_ADMIN"}, Drop: []v1.Capability{"ALL"}},
			SELinuxOptions:         &v1.SELinuxOptions{User: "u", Role: "r", Type: "t", Level: "s0"},
		},
	}

	return v1.Pod{
		Name: "app-7d9f8b6c5d-x2k9j", Namespace: "team-01",
		Spec: v1.PodSpec{
			NodeName:       "node-01",
			Containers:     []v1.Container{container},
			InitContainers: []v1.Container{{Name: "init", Image: "busybox:1.36"}},
			Volumes:        []v1.Volume{{Name: "data", EmptyDir: &v1.EmptyDirVolumeSource{}}},
		},
		Status: v1.PodStatus{
			Phase:      v1.PodRunning,
			PodIP:      "10.1.2.3",
			Conditions: []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionTrue}},
			ContainerStatuses: []v1.ContainerStatus{{
				Name: "app", Ready: true, RestartCount: 2,
				Image: "eu.gcr.io/project/app:v1.2.3", ContainerID: "containerd://abc",
				ImageID: "sha256:deadbeef",
				State:   v1.ContainerState{Running: &v1.ContainerStateRunning{StartedAt: metav1.Now()}},
			}},
		},
	}
}

// TestLoopersRowWidthMatchesHeaders pins the contract Table.AddRow panics on: a
// Looper must return exactly as many cells as it declares headers. Nothing
// checked it before, and a mismatch only showed up as a panic at runtime.
func TestLoopersRowWidthMatchesHeaders(t *testing.T) {
	pod := looperTestPod()
	container := pod.Spec.Containers[0]
	containerStatus := pod.Status.ContainerStatuses[0]

	loopers := map[string]Looper{
		"capabilities": &capabilities{},
		"commands":     &commands{},
		"conditions":   &conditions{},
		"environment":  &environment{},
		"image":        &image{},
		"lifecycle":    &lifecycle{},
		"ports":        &ports{},
		"probes":       &probes{},
		"resource":     &resource{ResourceType: "cpu"},
		"restarts":     &restarts{},
		"security":     &security{},
		"status":       &status{},
		"volumes":      &volumes{},
	}

	for name, loop := range loopers {
		t.Run(name, func(t *testing.T) {
			want := len(loop.Headers())
			if want == 0 {
				t.Fatal("Headers() is empty")
			}

			info := BuilderInformation{
				PodName:   pod.Name,
				Namespace: pod.Namespace,
				NodeName:  pod.Spec.NodeName,
				Name:      container.Name,
			}
			info.Data.pod = pod

			check := func(what string, rows [][]Cell, err error) {
				if err != nil {
					// several Loopers only support one of the three paths
					return
				}
				for i, row := range rows {
					if len(row) != want {
						t.Errorf("%s row %d has %d cells, Headers() declares %d", what, i, len(row), want)
					}
				}
			}

			specRows, err := loop.BuildContainerSpec(container, info)
			check("BuildContainerSpec", specRows, err)

			statusRows, err := loop.BuildContainerStatus(containerStatus, info)
			check("BuildContainerStatus", statusRows, err)

			podRows, err := loop.BuildPodRow(pod, info)
			check("BuildPodRow", podRows, err)

			// HideColumns must stay inside the declared header range
			for _, id := range loop.HideColumns(info) {
				if id < 0 || id >= want {
					t.Errorf("HideColumns returned %d, outside the %d headers", id, want)
				}
			}
		})
	}
}
