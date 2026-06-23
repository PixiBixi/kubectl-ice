package plugin

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// With -A, StatefulSet pods sharing a name across namespaces (thanos-receive-0)
// must not collide when resolving --pod-label / --annotation values. Keying by
// pod name alone made the last pod overwrite the others.
func TestGetPodLabelsCrossNamespace(t *testing.T) {
	pod := func(namespace, env string) v1.Pod {
		return v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "thanos-receive-0",
				Namespace: namespace,
				Labels:    map[string]string{"env": env},
			},
		}
	}

	c := &Connector{
		podList: []v1.Pod{pod("ns-a", "dev"), pod("ns-b", "preprod")},
	}

	got, err := c.GetPodLabels(c.podList)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 distinct entries (one per namespace), got %d: %v", len(got), got)
	}

	if v := got["ns-a/thanos-receive-0"]["env"]; v != "dev" {
		t.Errorf("ns-a label env = %q, want %q", v, "dev")
	}
	if v := got["ns-b/thanos-receive-0"]["env"]; v != "preprod" {
		t.Errorf("ns-b label env = %q, want %q", v, "preprod")
	}
}

func TestGetPodAnnotationsCrossNamespace(t *testing.T) {
	pod := func(namespace, team string) v1.Pod {
		return v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "thanos-receive-0",
				Namespace:   namespace,
				Annotations: map[string]string{"team": team},
			},
		}
	}

	c := &Connector{
		podList: []v1.Pod{pod("ns-a", "sre"), pod("ns-b", "platform")},
	}

	got, err := c.GetPodAnnotations(c.podList)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 distinct entries (one per namespace), got %d: %v", len(got), got)
	}

	if v := got["ns-a/thanos-receive-0"]["team"]; v != "sre" {
		t.Errorf("ns-a annotation team = %q, want %q", v, "sre")
	}
	if v := got["ns-b/thanos-receive-0"]["team"]; v != "platform" {
		t.Errorf("ns-b annotation team = %q, want %q", v, "platform")
	}
}
