package plugin

import (
	"testing"

	a1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// countingConnector wires a fake clientset and counts the list calls it sees, so
// a test can assert how many round trips a lookup pattern costs.
func countingConnector(t *testing.T, objects ...runtime.Object) (*Connector, func(resource string) int) {
	t.Helper()

	client := fake.NewClientset(objects...)
	lists := map[string]int{}
	client.PrependReactor("list", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
		lists[action.GetResource().Resource]++
		// fall through to the tracker, this reactor only counts
		return false, nil, nil
	})

	return &Connector{clientSet: client}, func(resource string) int { return lists[resource] }
}

func replicaSet(namespace, name string) *a1.ReplicaSet {
	return &a1.ReplicaSet{Name: name, Namespace: namespace}
}

// TestGetReplicaSetListsOncePerNamespace pins the lazy per namespace cache used
// when a single namespace is selected.
func TestGetReplicaSetListsOncePerNamespace(t *testing.T) {
	connect, listCount := countingConnector(t,
		replicaSet("team-a", "web-abc"),
		replicaSet("team-a", "api-def"),
	)

	for range 5 {
		if rs := connect.GetReplicaSet("web-abc", "team-a"); rs == nil {
			t.Fatal("web-abc not found")
		}
	}

	if got := listCount("replicasets"); got != 1 {
		t.Errorf("%d list calls for five lookups in one namespace, want 1", got)
	}
}

// TestGetReplicaSetAllNamespacesListsOnce is the fix: with -A one cluster wide
// list has to serve every namespace, where the old code listed each one.
func TestGetReplicaSetAllNamespacesListsOnce(t *testing.T) {
	connect, listCount := countingConnector(t,
		replicaSet("team-a", "web-abc"),
		replicaSet("team-b", "api-def"),
		replicaSet("team-c", "job-ghi"),
	)
	connect.Flags.allNamespaces = true

	for _, tc := range []struct{ name, namespace string }{
		{"web-abc", "team-a"}, {"api-def", "team-b"}, {"job-ghi", "team-c"},
	} {
		if rs := connect.GetReplicaSet(tc.name, tc.namespace); rs == nil {
			t.Fatalf("%s/%s not found", tc.namespace, tc.name)
		}
	}

	if got := listCount("replicasets"); got != 1 {
		t.Errorf("%d list calls across three namespaces, want 1", got)
	}
}

// TestGetReplicaSetMissDoesNotRelist covers the negative result. A namespace
// holding none of a kind used to list again on every lookup, because the empty
// result was never recorded.
func TestGetReplicaSetMissDoesNotRelist(t *testing.T) {
	connect, listCount := countingConnector(t, replicaSet("team-a", "web-abc"))

	for range 5 {
		if rs := connect.GetReplicaSet("nothing-here", "empty-namespace"); rs != nil {
			t.Fatal("found a ReplicaSet that does not exist")
		}
	}

	if got := listCount("replicasets"); got > 1 {
		t.Errorf("%d list calls for five misses in one namespace, want 1", got)
	}
}

// TestClearCacheDropsWorkloads is the watch refresh: leaving the workload caches
// behind froze the owner tree for the rest of the session.
func TestClearCacheDropsWorkloads(t *testing.T) {
	connect, listCount := countingConnector(t, replicaSet("team-a", "web-abc"))

	if rs := connect.GetReplicaSet("web-abc", "team-a"); rs == nil {
		t.Fatal("web-abc not found")
	}
	connect.ClearCache()
	if rs := connect.GetReplicaSet("web-abc", "team-a"); rs == nil {
		t.Fatal("web-abc not found after the cache was cleared")
	}

	if got := listCount("replicasets"); got != 2 {
		t.Errorf("%d list calls, want 2: the refresh has to refetch", got)
	}
}

func TestGetPodsUsesTheCache(t *testing.T) {
	connect, listCount := countingConnector(t,
		&v1.Pod{Name: "web-1", Namespace: "team-a"},
	)
	connect.SetNamespace("team-a")

	for range 3 {
		pods, err := connect.GetPods(nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(pods) != 1 {
			t.Fatalf("got %d pods, want 1", len(pods))
		}
	}

	if got := listCount("pods"); got != 1 {
		t.Errorf("%d list calls for three GetPods, want 1", got)
	}
}
