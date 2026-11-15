package chunk

import "testing"

func TestShards_MidLineLeadReleaseGolden(t *testing.T) {
	parent := Node{NodeID: "parent", Kind: "dieu", Path: "dieu-1", Text: "Lead"}
	child := Node{NodeID: "child", ParentNodeID: &parent.NodeID, Kind: "khoan", Path: "dieu-1/khoan-1", Text: "aa bb cccccc d\ne"}
	recipe := Recipe2()
	recipe.BodyLimit, recipe.MinimumRunes = 4, 1
	s := splitter{
		file:     File{Version: 1, DocumentKey: "vn:synthetic:1", Chunks: []Chunk{}},
		coverage: Coverage{Version: 1, DocumentKey: "vn:synthetic:1", Gaps: []Gap{}, Omissions: []Omission{}, CoveredRanges: []Range{}, WhitespaceRanges: []Range{}},
		meta:     Meta{Key: "vn:synthetic:1", Number: "1/TEST", Title: "Synthetic law"},
		recipe:   recipe,
		seen:     make(map[string]int),
	}
	owner := &treeNode{node: parent}
	node := &treeNode{node: child, parent: owner}
	s.shards(node, []carriedLead{{owner: owner, text: sourceText(parent)}}, sourceText(child))
	nodes := []Node{parent, child}
	s.finalCoverage(nodes)
	if err := s.checkCoverage(nodes); err != nil {
		t.Fatal(err)
	}
	assertJSONHash(t, "file", s.file, "592a04abb07dbcc34fd7d3a7d98b2b17c1a574ba755f27d35be46a278e67c2f8")
	assertJSONHash(t, "coverage", s.coverage, "ff9795920c6d66a06f4afe61313a9369c4b109df444ecee7b4b8f2d27f039039")
}
