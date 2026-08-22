package plugin

import (
	"fmt"
	"testing"
	"unsafe"

	v1 "k8s.io/api/core/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
)

// TestBuilderInformationSize guards the struct the Looper interface passes by
// value once per container. ParentData used to carry a Deployment, ReplicaSet,
// StatefulSet, DaemonSet, Job and CronJob that nothing ever read, which made
// this 8976 bytes. Only the Pod is read, and it is why the rest is 1168.
//
// Passing it by pointer instead was measured and is a regression: taking the
// address for an interface method call moves the struct to the heap, where the
// by-value copy was a stack memcpy. See the commit for the numbers.
func TestBuilderInformationSize(t *testing.T) {
	size := unsafe.Sizeof(BuilderInformation{})
	t.Logf("BuilderInformation is %d bytes, ParentData %d, v1.Pod %d",
		size, unsafe.Sizeof(ParentData{}), unsafe.Sizeof(v1.Pod{}))

	// a regression guard, not a spec: growth here is copied per container, per
	// Looper call, on every render
	if size > 2048 {
		t.Errorf("BuilderInformation grew to %d bytes, check what was added to ParentData", size)
	}
}

// benchPods builds a pod list shaped like a real workload: three containers, an
// init container, resources, ports, env and container statuses.
func benchPods(count int) []v1.Pod {
	pods := make([]v1.Pod, 0, count)
	for i := range count {
		pods = append(pods, v1.Pod{
			Name:      fmt.Sprintf("workload-%05d-abcde-xyz12", i),
			Namespace: fmt.Sprintf("team-%02d", i%20),
			Labels:    map[string]string{"app": "web", "version": "v1.2.3"},
			Spec: v1.PodSpec{
				NodeName: fmt.Sprintf("gke-node-pool-%02d", i%30),
				InitContainers: []v1.Container{
					{Name: "init-db", Image: "busybox:1.36"},
				},
				Containers: []v1.Container{
					{
						Name:  "app",
						Image: "eu.gcr.io/project/app:v1.2.3",
						Ports: []v1.ContainerPort{{ContainerPort: 8080, Name: "http"}},
						Resources: v1.ResourceRequirements{
							Requests: v1.ResourceList{"cpu": apiresource.MustParse("100m"), "memory": apiresource.MustParse("256Mi")},
							Limits:   v1.ResourceList{"cpu": apiresource.MustParse("500m"), "memory": apiresource.MustParse("512Mi")},
						},
						Env: []v1.EnvVar{{Name: "LOG_LEVEL", Value: "info"}},
					},
					{Name: "istio-proxy", Image: "docker.io/istio/proxyv2:1.20.0"},
				},
			},
			Status: v1.PodStatus{
				Phase: v1.PodRunning,
				InitContainerStatuses: []v1.ContainerStatus{
					{Name: "init-db", Ready: true, Image: "busybox:1.36"},
				},
				ContainerStatuses: []v1.ContainerStatus{
					{Name: "app", Ready: true, RestartCount: 2, Image: "eu.gcr.io/project/app:v1.2.3", ContainerID: "containerd://abc123"},
					{Name: "istio-proxy", Ready: true, ContainerID: "containerd://def456"},
				},
			},
		})
	}

	return pods
}

// benchmarkBuildContainerTable measures one full non-tree render: the pod loop,
// every Looper call and the row assembly, which is the path a watch mode
// refresh repeats on each pod event.
func benchmarkBuildContainerTable(b *testing.B, podCount int, loop Looper, spec bool) {
	pods := benchPods(podCount)

	for b.Loop() {
		builder := RowBuilder{
			LoopSpec:           spec,
			LoopStatus:         !spec,
			ShowInitContainers: true,
			Table:              &Table{},
		}
		info := BuilderInformation{}
		if err := builder.LoadHeaders(loop, &info); err != nil {
			b.Fatal(err)
		}
		if err := builder.BuildContainerTable(loop, &info, pods); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBuildImage200(b *testing.B)  { benchmarkBuildContainerTable(b, 200, &image{}, true) }
func BenchmarkBuildImage2000(b *testing.B) { benchmarkBuildContainerTable(b, 2000, &image{}, true) }
func BenchmarkBuildStatus2000(b *testing.B) {
	benchmarkBuildContainerTable(b, 2000, &status{}, false)
}
