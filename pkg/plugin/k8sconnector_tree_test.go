package plugin

import (
	"testing"

	a1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ownedPod(namespace, name, node, kind, owner string) *v1.Pod {
	return &v1.Pod{
		Name: name, Namespace: namespace,
		OwnerReferences: []metav1.OwnerReference{{Kind: kind, Name: owner}},
		Spec:            v1.PodSpec{NodeName: node},
	}
}

func countLeaves(nodes []*LeafNode) int {
	n := 0
	for _, c := range nodes {
		if len(c.child) == 0 {
			n++
		}
		n += countLeaves(c.child)
	}
	return n
}

// TestBuildOwnersListSameNameAcrossNamespaces: with -A two StatefulSets named
// db used to merge into one branch and one of the db-0 pods vanished.
func TestBuildOwnersListSameNameAcrossNamespaces(t *testing.T) {
	podA := ownedPod("a", "db-0", "n1", TypeNameStatefulSet, "db")
	podB := ownedPod("b", "db-0", "n1", TypeNameStatefulSet, "db")
	connect, _ := countingConnector(t,
		&a1.StatefulSet{Name: "db", Namespace: "a"},
		&a1.StatefulSet{Name: "db", Namespace: "b"},
	)
	connect.Flags.allNamespaces = true
	connect.podList = []v1.Pod{*podA, *podB}

	tree := connect.BuildOwnersList()
	if got := countLeaves(tree); got != 2 {
		t.Errorf("%d pod leaves in the tree, want 2", got)
	}
}

// TestGetReplicaSetWithSelect: --select used to skip the cache, so every pod
// listed again and every owner came back nil.
func TestGetReplicaSetWithSelect(t *testing.T) {
	connect, listCount := countingConnector(t, replicaSet("team-a", "web-abc"))
	connect.Flags.matchSpecList = map[string]matchValue{"HOSTNETWORK": {operator: "=", value: "false"}}

	for range 3 {
		if rs := connect.GetReplicaSet("web-abc", "team-a"); rs == nil {
			t.Fatal("web-abc not found with --select")
		}
	}
	if got := listCount("replicasets"); got != 1 {
		t.Errorf("%d list calls for three lookups, want 1", got)
	}
}

// TestGetDeploymentIgnoresPodSelector: -l selects pods, a Deployment's own
// labels often differ from its pod template.
func TestGetDeploymentIgnoresPodSelector(t *testing.T) {
	connect, _ := countingConnector(t,
		&a1.Deployment{Name: "web", Namespace: "team-a"},
		&a1.ReplicaSet{Name: "web-abc", Namespace: "team-a"},
	)
	connect.Flags.labels = "app=web"

	if d := connect.GetDeployment("web", "team-a"); d == nil {
		t.Error("Deployment not found when -l is set")
	}
	if rs := connect.GetReplicaSet("web-abc", "team-a"); rs == nil {
		t.Error("ReplicaSet not found when -l is set")
	}
}
