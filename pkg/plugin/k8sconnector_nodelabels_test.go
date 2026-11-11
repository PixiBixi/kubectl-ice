package plugin

import (
	"testing"

	v1 "k8s.io/api/core/v1"
)

// TestGetNodeLabelsSkipsUnscheduled: a pending pod has no node name, and the
// argument has to be used, not the connector cache (empty with -f).
func TestGetNodeLabelsSkipsUnscheduled(t *testing.T) {
	connect, _ := countingConnector(t,
		&v1.Node{Name: "n1", Labels: map[string]string{"zone": "a"}},
	)
	// the cache holds only a pending pod: reading it instead of the argument
	// meant Get("") and a failed command
	connect.podList = []v1.Pod{{Spec: v1.PodSpec{}}}
	pods := []v1.Pod{{Spec: v1.PodSpec{NodeName: "n1"}}, {Spec: v1.PodSpec{}}}

	labels, err := connect.GetNodeLabels(pods)
	if err != nil {
		t.Fatal(err)
	}
	if labels["n1"]["zone"] != "a" {
		t.Errorf("labels = %v, want n1 zone=a", labels)
	}

	labels, err = connect.GetNodeLabels([]v1.Pod{{Spec: v1.PodSpec{}}})
	if err != nil || len(labels) != 0 {
		t.Errorf("only a pending pod: labels=%v err=%v, want empty and no error", labels, err)
	}
}
