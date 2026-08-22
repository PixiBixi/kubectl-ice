package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeMultiDocYaml writes count pod documents separated by ---, the shape
// `kubectl get pod x -o yaml` produces one at a time.
func writeMultiDocYaml(tb testing.TB, count int) string {
	tb.Helper()

	var doc strings.Builder
	for i := range count {
		if i > 0 {
			doc.WriteString("---\n")
		}
		fmt.Fprintf(&doc, `apiVersion: v1
kind: Pod
metadata:
  name: workload-%05d
  namespace: team-01
spec:
  containers:
  - name: app
    image: eu.gcr.io/project/app:v1.2.3
    resources:
      requests:
        cpu: 100m
        memory: 256Mi
  - name: istio-proxy
    image: docker.io/istio/proxyv2:1.20.0
`, i)
	}

	path := filepath.Join(tb.TempDir(), "pods.yaml")
	if err := os.WriteFile(path, []byte(doc.String()), 0o600); err != nil {
		tb.Fatal(err)
	}

	return path
}

// writeOneBigDoc writes a single pod document with lineCount lines of env vars.
// This is the shape that hurt: the line buffer grew by reassignment, so cost was
// quadratic in the lines of one document, and `kubectl get pods -o yaml` is one
// document of tens of thousands of lines.
func writeOneBigDoc(tb testing.TB, envCount int) string {
	tb.Helper()

	var doc strings.Builder
	doc.WriteString(`apiVersion: v1
kind: Pod
metadata:
  name: workload-00001
  namespace: team-01
spec:
  containers:
  - name: app
    image: eu.gcr.io/project/app:v1.2.3
    env:
`)
	for i := range envCount {
		fmt.Fprintf(&doc, "    - name: SETTING_%05d\n      value: \"some value %05d\"\n", i, i)
	}

	path := filepath.Join(tb.TempDir(), "onepod.yaml")
	if err := os.WriteFile(path, []byte(doc.String()), 0o600); err != nil {
		tb.Fatal(err)
	}

	return path
}

func BenchmarkLoadYamlOneBigDoc(b *testing.B) {
	for _, envCount := range []int{500, 5000} {
		b.Run(fmt.Sprintf("%dlines", envCount*2), func(b *testing.B) {
			path := writeOneBigDoc(b, envCount)
			builder := RowBuilder{}

			for b.Loop() {
				if _, err := builder.loadYaml(path); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkLoadYaml is the --filename path. The line buffer used to grow by
// reassignment, so a large document cost O(n^2) in the number of lines.
func BenchmarkLoadYaml(b *testing.B) {
	for _, count := range []int{50, 500} {
		b.Run(fmt.Sprintf("%dpods", count), func(b *testing.B) {
			path := writeMultiDocYaml(b, count)
			builder := RowBuilder{}

			for b.Loop() {
				pods, err := builder.loadYaml(path)
				if err != nil {
					b.Fatal(err)
				}
				if len(pods) != count {
					b.Fatalf("got %d pods, want %d", len(pods), count)
				}
			}
		})
	}
}
