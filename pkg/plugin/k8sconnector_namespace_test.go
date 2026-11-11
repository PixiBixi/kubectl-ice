package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func writeKubeconfig(t *testing.T, namespace string) string {
	t.Helper()
	content := `apiVersion: v1
kind: Config
current-context: ctx
clusters:
- name: c
  cluster:
    server: https://127.0.0.1:6443
users:
- name: u
  user:
    token: x
contexts:
- name: ctx
  context:
    cluster: c
    user: u
    namespace: ` + namespace + "\n"
	path := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestGetNamespaceUsesKubeconfigFlag: the namespace has to come from the
// --kubeconfig the client was built from, not from $KUBECONFIG.
func TestGetNamespaceUsesKubeconfigFlag(t *testing.T) {
	t.Setenv("KUBECONFIG", writeKubeconfig(t, "from-env"))
	flags := genericclioptions.NewConfigFlags(false)
	path := writeKubeconfig(t, "from-flag")
	flags.KubeConfig = &path

	connect := Connector{configFlags: flags}
	if got := connect.GetNamespace(false); got != "from-flag" {
		t.Errorf("namespace = %q, want from-flag", got)
	}
}

func TestGetNamespaceBrokenKubeconfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(path, []byte("{{ not yaml"), 0o600); err != nil {
		t.Fatal(err)
	}
	flags := genericclioptions.NewConfigFlags(false)
	flags.KubeConfig = &path

	connect := Connector{configFlags: flags}
	if got := connect.GetNamespace(false); got != "default" {
		t.Errorf("namespace = %q, want default", got)
	}
}
