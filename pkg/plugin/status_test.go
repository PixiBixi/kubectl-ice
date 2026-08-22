package plugin

import "testing"

// TestTrimStatusMessage covers the strings.SplitSeq / strings.Builder rewrite of
// the message trimmer, which drops the container= and pod= noise kubelet adds.
func TestTrimStatusMessage(t *testing.T) {
	tests := []struct {
		name          string
		message       string
		podName       string
		containerName string
		want          string
	}{
		{
			name:          "strips container and pod tokens",
			message:       "Back-off restarting failed container=app pod=web-7d9f8b6c5d-x2k9j_default(abc-123) restarting",
			podName:       "web-7d9f8b6c5d-x2k9j",
			containerName: "app",
			want:          "Back-off restarting failed restarting",
		},
		{
			name:          "keeps text when nothing matches",
			message:       "short",
			podName:       "p",
			containerName: "c",
			want:          "short",
		},
		{
			name:          "empty message",
			message:       "",
			podName:       "p",
			containerName: "c",
			want:          "",
		},
		{
			name:          "empty container name",
			message:       "some message",
			podName:       "p",
			containerName: "",
			want:          "",
		},
	}

	s := &status{}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := s.trimStatusMessage(test.message, test.podName, test.containerName); got != test.want {
				t.Errorf("want %q, got %q", test.want, got)
			}
		})
	}
}

func BenchmarkTrimStatusMessage(b *testing.B) {
	s := &status{}
	message := "Readiness probe failed: HTTP probe failed with statuscode: 503 for container=sidecar in pod=api-5f7b_prod(def)"

	for b.Loop() {
		s.trimStatusMessage(message, "api-5f7b", "sidecar")
	}
}
