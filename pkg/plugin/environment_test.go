package plugin

import (
	"testing"

	v1 "k8s.io/api/core/v1"
)

// TestGetConfigMapValuePodNamespace: with -A the ConfigMap was fetched with an
// empty namespace and the error swallowed.
func TestGetConfigMapValuePodNamespace(t *testing.T) {
	connect, _ := countingConnector(t,
		&v1.ConfigMap{Name: "cfg", Namespace: "team-a", Data: map[string]string{"k": "a"}},
		&v1.ConfigMap{Name: "cfg", Namespace: "team-b", Data: map[string]string{"k": "b"}},
	)
	connect.Flags.allNamespaces = true

	for ns, want := range map[string]string{"team-a": "a", "team-b": "b"} {
		got, err := connect.GetConfigMapValue(ns, "cfg", "k")
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s/cfg k = %q, want %q", ns, got, want)
		}
	}

	if _, err := connect.GetConfigMapValue("team-a", "missing", "k"); err == nil {
		t.Error("missing ConfigMap returned no error")
	}

	env := environment{Connection: connect}
	row := env.envBuildRow(BuilderInformation{Namespace: "team-b"}, v1.EnvVar{
		Name:      "K",
		ValueFrom: &v1.EnvVarSource{ConfigMapKeyRef: &v1.ConfigMapKeySelector{Name: "missing", Key: "k"}},
	}, connect, true)
	if row[1].text != "<not found>" {
		t.Errorf("env cell = %q, want <not found>", row[1].text)
	}
}
