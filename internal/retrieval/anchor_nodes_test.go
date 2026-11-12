package retrieval

import (
	"context"
	"testing"
)

func TestAnchorNodes_Version4(t *testing.T) {
	e := &Engine{release: Release{SchemaVersion: 4}}
	nodes, err := e.AnchorNodes(context.Background(), []AnchorRef{{DocumentKey: "vn:1", Anchor: "dieu-1"}})
	if err != nil || len(nodes) != 0 {
		t.Fatalf("nodes %v, error %v", nodes, err)
	}
}
