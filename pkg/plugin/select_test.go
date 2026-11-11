package plugin

import (
	"testing"

	v1 "k8s.io/api/core/v1"
)

// TestSelectMatchingPodSpecAnd: several --select terms must all match, a pod
// is listed once, and an unknown field is an error.
func TestSelectMatchingPodSpecAnd(t *testing.T) {
	pods := []v1.Pod{
		{Name: "both", Spec: v1.PodSpec{HostNetwork: true, DNSPolicy: v1.DNSClusterFirst}},
		{Name: "one", Spec: v1.PodSpec{HostNetwork: true, DNSPolicy: v1.DNSDefault}},
	}
	connect := Connector{}
	var err error
	connect.Flags.matchSpecList, err = splitAndFilterMatchList("hostNetwork==true,dnsPolicy==ClusterFirst", selectAllowedChars, []string{"!=", "==", "="})
	if err != nil {
		t.Fatal(err)
	}
	got, err := connect.SelectMatchinghPodSpec(pods)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "both" {
		t.Errorf("got %d pods, want only 'both'", len(got))
	}

	connect.Flags.matchSpecList = map[string]matchValue{"NOSUCHFIELD": {operator: "=", value: "x"}}
	if _, err := connect.SelectMatchinghPodSpec(pods); err == nil {
		t.Error("unknown field returned no error")
	}

	if _, err := splitAndFilterMatchList("nodeName=ip-10-0-0-1.ec2.internal", selectAllowedChars, []string{"!=", "==", "="}); err != nil {
		t.Errorf("a dotted node name is rejected: %v", err)
	}
}
