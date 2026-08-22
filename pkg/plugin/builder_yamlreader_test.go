package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const podYaml = `apiVersion: v1
kind: Pod
metadata:
  name: nginx
spec:
  containers:
  - name: web
    image: nginx:1.27
`

// listYaml is what `kubectl get pods -o yaml` produces: one document wrapping
// every item rather than one document per pod.
const listYaml = `apiVersion: v1
kind: List
items:
- apiVersion: v1
  kind: Pod
  metadata:
    name: nginx
  spec:
    containers:
    - name: web
      image: nginx:1.27
`

const serviceYaml = `apiVersion: v1
kind: Service
metadata:
  name: web
spec:
  ports:
  - port: 80
`

func TestLoadYamlFromFile(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantPods  int
		wantError bool
	}{
		{name: "single pod", content: podYaml, wantPods: 1},
		{name: "two pods separated by ---", content: podYaml + "---\n" + podYaml, wantPods: 2},
		{name: "empty input", content: "", wantError: true},
		{name: "whitespace only", content: "\n\n", wantError: true},
		{name: "a List contributes its items", content: listYaml, wantPods: 1},
		{name: "unsupported kind", content: serviceYaml, wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "input.yaml")
			if err := os.WriteFile(path, []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}

			builder := RowBuilder{}
			pods, err := builder.loadYaml(path)

			if test.wantError {
				if err == nil {
					t.Fatalf("want an error, got %d pods", len(pods))
				}
				if !strings.Contains(err.Error(), "no pod found") {
					t.Errorf("want a no pod found error, got %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(pods) != test.wantPods {
				t.Fatalf("want %d pods, got %d", test.wantPods, len(pods))
			}
			if pods[0].Name != "nginx" {
				t.Errorf("want pod name nginx, got %q", pods[0].Name)
			}
			if len(pods[0].Spec.Containers) != 1 {
				t.Errorf("want 1 container, got %d", len(pods[0].Spec.Containers))
			}
		})
	}
}
