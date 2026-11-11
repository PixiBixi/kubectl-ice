package plugin

import (
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// TestGetAllPodsAllNamespacesFiltersAndPages: ice node used to fetch every pod
// object of the cluster in one unbounded response.
func TestGetAllPodsAllNamespacesFiltersAndPages(t *testing.T) {
	client := fake.NewClientset()
	var selectors []string
	var limits []int64
	client.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		la, ok := action.(k8stesting.ListActionImpl)
		if !ok {
			return false, nil, nil
		}
		selectors = append(selectors, la.ListOptions.FieldSelector)
		limits = append(limits, la.ListOptions.Limit)
		if la.ListOptions.Continue == "" {
			return true, &v1.PodList{
				Continue: "next",
				Items:    []v1.Pod{{Name: "p1"}},
			}, nil
		}
		return true, &v1.PodList{Items: []v1.Pod{{Name: "p2"}}}, nil
	})
	connect := Connector{clientSet: client}

	pods, err := connect.GetAllPodsAllNamespaces([]string{"n1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(pods) != 2 {
		t.Fatalf("got %d pods over two pages, want 2", len(pods))
	}
	for i, sel := range selectors {
		if !strings.Contains(sel, "spec.nodeName=n1") || !strings.Contains(sel, "status.phase!=Succeeded") {
			t.Errorf("field selector %q misses the node or phase filter", sel)
		}
		if limits[i] == 0 {
			t.Error("list is not paginated")
		}
	}
}
