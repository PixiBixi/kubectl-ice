package plugin

import (
	"testing"

	v1 "k8s.io/api/core/v1"
)

// cellTexts flattens a built row to its rendered text, so a test can assert on
// what the user sees without reaching into the Cell internals.
func cellTexts(cells []Cell) []string {
	out := make([]string, len(cells))
	for i, c := range cells {
		out[i] = c.text
	}
	return out
}

// TestSecurityBuildRowContainerOnly covers a container that sets runAsGroup on a
// pod that does not. securityBuildRow used to test csc.RunAsGroup and then
// dereference psc.RunAsGroup, so this input segfaulted `ice security`.
func TestSecurityBuildRowContainerOnly(t *testing.T) {
	sec := &security{}
	csc := &v1.SecurityContext{
		RunAsGroup:   new(int64(2000)),
		RunAsUser:    new(int64(1000)),
		RunAsNonRoot: new(true),
	}

	for _, test := range []struct {
		name string
		psc  *v1.PodSecurityContext
	}{
		{name: "pod has no security context", psc: nil},
		{name: "pod security context sets nothing", psc: &v1.PodSecurityContext{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := cellTexts(sec.securityBuildRow(BuilderInformation{}, csc, test.psc))

			// order is allowPrivilegeEscalation, privileged, readOnlyRootFilesystem,
			// runAsNonRoot, runAsUser, runAsGroup
			if len(got) != 6 {
				t.Fatalf("want 6 cells, got %d: %v", len(got), got)
			}
			if got[5] != "2000" {
				t.Errorf("runAsGroup: want the container value 2000, got %q", got[5])
			}
			if got[4] != "1000" {
				t.Errorf("runAsUser: want 1000, got %q", got[4])
			}
		})
	}
}

// TestSeLinuxBuildRowContainerOverride covers the container overriding the pod
// SELinux options. seLinuxBuildRow used to read psc.SELinuxOptions inside the
// csc branch, which both crashed on a pod without a security context and made
// the container values unreachable.
func TestSeLinuxBuildRowContainerOverride(t *testing.T) {
	sec := &security{}
	csc := &v1.SecurityContext{
		SELinuxOptions: &v1.SELinuxOptions{
			User: "container_u", Role: "container_r", Type: "container_t", Level: "s0:c1",
		},
	}

	tests := []struct {
		name string
		psc  *v1.PodSecurityContext
	}{
		{name: "pod has no security context", psc: nil},
		{name: "pod has no selinux options", psc: &v1.PodSecurityContext{}},
		{
			name: "pod selinux options are overridden",
			psc: &v1.PodSecurityContext{SELinuxOptions: &v1.SELinuxOptions{
				User: "pod_u", Role: "pod_r", Type: "pod_t", Level: "s0:c9",
			}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := cellTexts(sec.seLinuxBuildRow(BuilderInformation{}, csc, test.psc))

			// order is user, role, type, level
			want := []string{"container_u", "container_r", "container_t", "s0:c1"}
			if len(got) != len(want) {
				t.Fatalf("want %d cells, got %d: %v", len(want), len(got), got)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("cell %d: want %q, got %q", i, want[i], got[i])
				}
			}
		})
	}
}

// TestSeLinuxBuildRowPodOnly checks the pod values still show through when the
// container sets nothing, which is the path the override must not break.
func TestSeLinuxBuildRowPodOnly(t *testing.T) {
	sec := &security{}
	psc := &v1.PodSecurityContext{SELinuxOptions: &v1.SELinuxOptions{
		User: "pod_u", Role: "pod_r", Type: "pod_t", Level: "s0:c9",
	}}

	got := cellTexts(sec.seLinuxBuildRow(BuilderInformation{}, &v1.SecurityContext{}, psc))

	want := []string{"pod_u", "pod_r", "pod_t", "s0:c9"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("cell %d: want %q, got %q", i, want[i], got[i])
		}
	}
}
