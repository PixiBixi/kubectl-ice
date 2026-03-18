package plugin

import (
	"testing"
)

// ***********************
// LeafNode.getChild
// ***********************

func TestLeafNodeGetChildCreatesNew(t *testing.T) {
	root := &LeafNode{child: []*LeafNode{}, childIndex: make(map[string]*LeafNode)}

	child := root.getChild("pod-1")
	if child == nil {
		t.Fatal("getChild returned nil")
	}
	if child.name != "pod-1" {
		t.Errorf("child.name = %q, want %q", child.name, "pod-1")
	}
	if len(root.child) != 1 {
		t.Errorf("root has %d children, want 1", len(root.child))
	}
}

func TestLeafNodeGetChildReturnsSame(t *testing.T) {
	root := &LeafNode{child: []*LeafNode{}, childIndex: make(map[string]*LeafNode)}

	a := root.getChild("pod-1")
	b := root.getChild("pod-1")

	if a != b {
		t.Error("getChild should return the same pointer for the same name")
	}
	if len(root.child) != 1 {
		t.Errorf("root has %d children, want 1 (no duplicate)", len(root.child))
	}
}

func TestLeafNodeGetChildDistinctChildren(t *testing.T) {
	root := &LeafNode{child: []*LeafNode{}, childIndex: make(map[string]*LeafNode)}

	root.getChild("pod-a")
	root.getChild("pod-b")
	root.getChild("pod-c")

	if len(root.child) != 3 {
		t.Errorf("root has %d children, want 3", len(root.child))
	}
	if len(root.childIndex) != 3 {
		t.Errorf("childIndex has %d entries, want 3", len(root.childIndex))
	}
}

func TestLeafNodeGetChildNilIndex(t *testing.T) {
	// getChild must handle a nil childIndex (lazy init)
	root := &LeafNode{child: []*LeafNode{}}

	child := root.getChild("pod-1")
	if child == nil {
		t.Fatal("getChild returned nil with nil childIndex")
	}
	// second call must reuse the existing entry
	same := root.getChild("pod-1")
	if child != same {
		t.Error("getChild should return same pointer after lazy init")
	}
}

func TestLeafNodeGetChildDeepTree(t *testing.T) {
	// Simulate a realistic ownership chain: root → deploy → rs → pod
	root := &LeafNode{child: []*LeafNode{}, childIndex: make(map[string]*LeafNode)}

	deploy := root.getChild("my-deploy")
	rs := deploy.getChild("my-rs-abc12")
	pod := rs.getChild("my-pod-xyz99")

	if pod.name != "my-pod-xyz99" {
		t.Errorf("leaf name = %q, want %q", pod.name, "my-pod-xyz99")
	}
	// re-fetching intermediate nodes must return the same pointers
	if root.getChild("my-deploy") != deploy {
		t.Error("deploy pointer mismatch on second getChild call")
	}
	if deploy.getChild("my-rs-abc12") != rs {
		t.Error("rs pointer mismatch on second getChild call")
	}
}
