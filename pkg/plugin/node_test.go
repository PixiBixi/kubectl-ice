package plugin

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// *****************
// pctOf
// *****************
type pctOfTest struct {
	value   int64
	total   int64
	wantStr string
	wantPct float64
}

var pctOfTests = []pctOfTest{
	{0, 0, "-", 0},
	{0, 100, "0.0", 0},
	{50, 100, "50.0", 50},
	{100, 100, "100.0", 100},
	{1, 3, "33.3", 33.333333333333336},
	{200, 100, "200.0", 200},
}

func TestPctOf(t *testing.T) {
	for _, tt := range pctOfTests {
		gotStr, gotPct := pctOf(tt.value, tt.total)
		if gotStr != tt.wantStr {
			t.Errorf("pctOf(%d,%d) str = %q, want %q", tt.value, tt.total, gotStr, tt.wantStr)
		}
		if tt.total != 0 && !floatClose(gotPct, tt.wantPct, 0.001) {
			t.Errorf("pctOf(%d,%d) pct = %f, want %f", tt.value, tt.total, gotPct, tt.wantPct)
		}
	}
}

func floatClose(a, b, tol float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= tol
}

// *****************
// nodeReadyStatus
// *****************
type nodeReadyStatusTest struct {
	node     v1.Node
	expected string
}

var nodeReadyStatusTests = []nodeReadyStatusTest{
	{
		v1.Node{Status: v1.NodeStatus{Conditions: []v1.NodeCondition{
			{Type: v1.NodeReady, Status: v1.ConditionTrue},
		}}},
		"Ready",
	},
	{
		v1.Node{Status: v1.NodeStatus{Conditions: []v1.NodeCondition{
			{Type: v1.NodeReady, Status: v1.ConditionFalse},
		}}},
		"NotReady",
	},
	{
		v1.Node{Status: v1.NodeStatus{Conditions: []v1.NodeCondition{
			{Type: v1.NodeReady, Status: v1.ConditionUnknown},
		}}},
		"NotReady",
	},
	{
		// unschedulable overrides ready status
		v1.Node{
			Spec: v1.NodeSpec{Unschedulable: true},
			Status: v1.NodeStatus{Conditions: []v1.NodeCondition{
				{Type: v1.NodeReady, Status: v1.ConditionTrue},
			}},
		},
		"SchedulingDisabled",
	},
	{
		// no conditions at all
		v1.Node{},
		"Unknown",
	},
}

func TestNodeReadyStatus(t *testing.T) {
	for _, tt := range nodeReadyStatusTests {
		if got := nodeReadyStatus(tt.node); got != tt.expected {
			t.Errorf("nodeReadyStatus() = %q, want %q", got, tt.expected)
		}
	}
}

// *****************
// nodeRoles
// *****************
type nodeRolesTest struct {
	labels   map[string]string
	expected string
}

var nodeRolesTests = []nodeRolesTest{
	{map[string]string{"node-role.kubernetes.io/worker": ""}, "worker"},
	{map[string]string{"node-role.kubernetes.io/control-plane": ""}, "control-plane"},
	{map[string]string{"kubernetes.io/role": "master"}, "master"},
	{map[string]string{}, "<none>"},
	{nil, "<none>"},
}

func TestNodeRoles(t *testing.T) {
	for _, tt := range nodeRolesTests {
		node := v1.Node{ObjectMeta: metav1.ObjectMeta{Labels: tt.labels}}
		if got := nodeRoles(node); got != tt.expected {
			t.Errorf("nodeRoles(%v) = %q, want %q", tt.labels, got, tt.expected)
		}
	}
}

// ***********************
// computeNodeAllocations
// ***********************
func TestComputeNodeAllocations(t *testing.T) {
	cpu100m := apiresource.MustParse("100m")
	cpu200m := apiresource.MustParse("200m")
	mem128Mi := apiresource.MustParse("128Mi")
	mem256Mi := apiresource.MustParse("256Mi")

	pods := []v1.Pod{
		{
			Spec: v1.PodSpec{
				NodeName: "node1",
				Containers: []v1.Container{
					{Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceCPU:    cpu100m,
							v1.ResourceMemory: mem128Mi,
						},
						Limits: v1.ResourceList{
							v1.ResourceCPU:    cpu200m,
							v1.ResourceMemory: mem256Mi,
						},
					}},
				},
			},
			Status: v1.PodStatus{Phase: v1.PodRunning},
		},
		{
			// completed pods must be excluded
			Spec:   v1.PodSpec{NodeName: "node1"},
			Status: v1.PodStatus{Phase: v1.PodSucceeded},
		},
		{
			// unscheduled pods (no NodeName) must be excluded
			Spec:   v1.PodSpec{NodeName: ""},
			Status: v1.PodStatus{Phase: v1.PodRunning},
		},
	}

	allocs := computeNodeAllocations(pods)

	a, ok := allocs["node1"]
	if !ok {
		t.Fatal("expected allocation entry for node1")
	}
	if a.podCount != 1 {
		t.Errorf("podCount = %d, want 1", a.podCount)
	}
	if a.cpuRequested != 100 {
		t.Errorf("cpuRequested = %d, want 100m", a.cpuRequested)
	}
	if a.cpuLimit != 200 {
		t.Errorf("cpuLimit = %d, want 200m", a.cpuLimit)
	}
	wantMem128 := mem128Mi.Value()
	if a.memRequested != wantMem128 {
		t.Errorf("memRequested = %d, want %d", a.memRequested, wantMem128)
	}
	wantMem256 := mem256Mi.Value()
	if a.memLimit != wantMem256 {
		t.Errorf("memLimit = %d, want %d", a.memLimit, wantMem256)
	}
}

func TestComputeNodeAllocationsMultiplePods(t *testing.T) {
	cpu100m := apiresource.MustParse("100m")

	pods := []v1.Pod{
		{
			Spec: v1.PodSpec{
				NodeName:   "node1",
				Containers: []v1.Container{{Resources: v1.ResourceRequirements{Requests: v1.ResourceList{v1.ResourceCPU: cpu100m}}}},
			},
			Status: v1.PodStatus{Phase: v1.PodRunning},
		},
		{
			Spec: v1.PodSpec{
				NodeName:   "node1",
				Containers: []v1.Container{{Resources: v1.ResourceRequirements{Requests: v1.ResourceList{v1.ResourceCPU: cpu100m}}}},
			},
			Status: v1.PodStatus{Phase: v1.PodRunning},
		},
		{
			Spec: v1.PodSpec{
				NodeName:   "node2",
				Containers: []v1.Container{{Resources: v1.ResourceRequirements{Requests: v1.ResourceList{v1.ResourceCPU: cpu100m}}}},
			},
			Status: v1.PodStatus{Phase: v1.PodRunning},
		},
	}

	allocs := computeNodeAllocations(pods)

	if allocs["node1"].podCount != 2 {
		t.Errorf("node1 podCount = %d, want 2", allocs["node1"].podCount)
	}
	if allocs["node1"].cpuRequested != 200 {
		t.Errorf("node1 cpuRequested = %d, want 200", allocs["node1"].cpuRequested)
	}
	if allocs["node2"].podCount != 1 {
		t.Errorf("node2 podCount = %d, want 1", allocs["node2"].podCount)
	}
}
