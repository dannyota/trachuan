package build

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
)

func TestTree_PreorderAndInvalidAncestry(t *testing.T) {
	parent := "root"
	cases := []struct {
		name    string
		nodes   []chunk.Node
		invalid bool
	}{
		{"tree", []chunk.Node{{NodeID: "root", Ordinal: 1, Path: "a"}, {NodeID: "child", ParentNodeID: &parent, Ordinal: 1, Path: "b"}}, false},
		{"parent after child", []chunk.Node{{NodeID: "child", ParentNodeID: &parent, Ordinal: 1, Path: "b"}, {NodeID: "root", Ordinal: 1, Path: "a"}}, true},
		{"duplicate", []chunk.Node{{NodeID: "root", Ordinal: 1, Path: "a"}, {NodeID: "root", Ordinal: 2, Path: "b"}}, true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := preorder(tt.nodes)
			if (err != nil) != tt.invalid {
				t.Fatalf("got %v", err)
			}
		})
	}
}
