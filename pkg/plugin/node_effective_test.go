package plugin

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
)

func cpuReq(req string) v1.ResourceRequirements {
	return v1.ResourceRequirements{
		Requests: v1.ResourceList{v1.ResourceCPU: apiresource.MustParse(req)},
		Limits:   v1.ResourceList{v1.ResourceCPU: apiresource.MustParse(req)},
	}
}

// TestComputeNodeAllocationsEffectiveRequest pins the kubelet rule: the init
// container max is per pod, sidecars add up, and overhead is counted.
func TestComputeNodeAllocationsEffectiveRequest(t *testing.T) {
	always := v1.ContainerRestartPolicyAlways
	running := v1.PodStatus{Phase: v1.PodRunning}
	pods := []v1.Pod{
		{
			Spec:   v1.PodSpec{NodeName: "node1", Containers: []v1.Container{{Resources: cpuReq("2000m")}}},
			Status: running,
		},
		{
			Spec: v1.PodSpec{
				NodeName:       "node1",
				InitContainers: []v1.Container{{Resources: cpuReq("3000m")}},
				Containers:     []v1.Container{{Resources: cpuReq("100m")}},
			},
			Status: running,
		},
		{
			// app 100m + sidecar 200m = 300m, init 500m + sidecar started before it = 700m, + 50m overhead
			Spec: v1.PodSpec{
				NodeName: "node2",
				InitContainers: []v1.Container{
					{Resources: cpuReq("200m"), RestartPolicy: &always},
					{Resources: cpuReq("500m")},
				},
				Containers: []v1.Container{{Resources: cpuReq("100m")}},
				Overhead:   v1.ResourceList{v1.ResourceCPU: apiresource.MustParse("50m")},
			},
			Status: running,
		},
		{
			// sidecar after the init container: the init container does not see it
			Spec: v1.PodSpec{
				NodeName: "node3",
				InitContainers: []v1.Container{
					{Resources: cpuReq("500m")},
					{Resources: cpuReq("200m"), RestartPolicy: &always},
				},
				Containers: []v1.Container{{Resources: cpuReq("100m")}},
			},
			Status: running,
		},
	}

	allocs := computeNodeAllocations(pods)

	for node, want := range map[string]int64{"node1": 5000, "node2": 750, "node3": 500} {
		if got := allocs[node].cpuRequested; got != want {
			t.Errorf("%s cpuRequested = %dm, want %dm", node, got, want)
		}
		if got := allocs[node].cpuLimit; got != want {
			t.Errorf("%s cpuLimit = %dm, want %dm", node, got, want)
		}
	}
}
