package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadYamlLongLine: last-applied-configuration annotations go past the
// default 64 KiB scanner line.
func TestLoadYamlLongLine(t *testing.T) {
	content := "apiVersion: v1\nkind: Pod\nmetadata:\n  name: nginx\n  annotations:\n    big: " +
		strings.Repeat("x", 200*1024) + "\n" + "spec:\n  containers:\n  - name: web\n    image: nginx:1.27\n"
	path := filepath.Join(t.TempDir(), "input.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	builder := RowBuilder{}
	pods, err := builder.loadYaml(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(pods) != 1 {
		t.Errorf("got %d pods, want 1", len(pods))
	}
}
