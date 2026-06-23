package plugin

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

// Two StatefulSet pods sharing the same name across different namespaces
// (e.g. thanos-receive-0) must NOT collide in the metrics hashtable when
// using -A. Keying by pod name alone made the last pod overwrite the others,
// so every row showed identical usage.
func TestPodMetrics2HashtableCrossNamespace(t *testing.T) {
	metric := func(namespace string, usage string) v1beta1.PodMetrics {
		return v1beta1.PodMetrics{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "thanos-receive-0",
				Namespace: namespace,
			},
			Containers: []v1beta1.ContainerMetrics{
				{
					Name: "receive",
					Usage: v1.ResourceList{
						v1.ResourceMemory: apiresource.MustParse(usage),
					},
				},
			},
		}
	}

	stateList := []v1beta1.PodMetrics{
		metric("ns-a", "50Mi"),
		metric("ns-b", "200Mi"),
	}

	s := &resource{}
	got := s.podMetrics2Hashtable(stateList)

	if len(got) != 2 {
		t.Fatalf("expected 2 distinct entries (one per namespace), got %d: %v", len(got), got)
	}

	keyA := "ns-a/thanos-receive-0"
	keyB := "ns-b/thanos-receive-0"

	usageA := got[keyA]["receive"][v1.ResourceMemory]
	usageB := got[keyB]["receive"][v1.ResourceMemory]

	wantA := apiresource.MustParse("50Mi")
	wantB := apiresource.MustParse("200Mi")

	if usageA.Cmp(wantA) != 0 {
		t.Errorf("namespace ns-a: expected %s, got %s", wantA.String(), usageA.String())
	}
	if usageB.Cmp(wantB) != 0 {
		t.Errorf("namespace ns-b: expected %s, got %s", wantB.String(), usageB.String())
	}
}
