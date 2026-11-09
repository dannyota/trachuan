package chunk

import (
	"strings"
	"testing"
)

func TestSplit_LeadWithNoRoomForWordKeepsParentOwner(t *testing.T) {
	parent := "article"
	lead := strings.Repeat("l", 900)
	word := strings.Repeat("w", 200)
	nodes := []Node{
		{NodeID: parent, Kind: "dieu", Path: "dieu-1", Text: lead},
		{NodeID: "child", ParentNodeID: &parent, Kind: "khoan", Path: "dieu-1/khoan-1", Text: word},
	}
	file, _, err := Split(nodes, Meta{Key: "vn:vbpl:1"}, Recipe1())
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Chunks) != 2 || file.Chunks[0].NodeID != parent || file.Chunks[0].Content != lead || file.Chunks[1].NodeID != "child" || file.Chunks[1].Content != word {
		t.Fatalf("unexpected chunks: %#v", file.Chunks)
	}
}

func TestSplit_NestedLeadsKeepTheirOwners(t *testing.T) {
	for _, tc := range []struct {
		name        string
		articleLead string
	}{
		{name: "combined leads leave no room", articleLead: strings.Repeat("a", 30)},
		{name: "article lead exceeds limit", articleLead: strings.Repeat("a", 70)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			article, clause := "article", "clause"
			clauseLead := strings.Repeat("c", 25)
			nodes := []Node{
				{NodeID: article, Kind: "dieu", Path: "dieu-1", Text: tc.articleLead},
				{NodeID: clause, ParentNodeID: &article, Kind: "khoan", Path: "dieu-1/khoan-1", Text: clauseLead},
				{NodeID: "point-1", ParentNodeID: &clause, Kind: "diem", Path: "dieu-1/khoan-1/diem-a", Text: strings.Repeat("p", 20)},
				{NodeID: "point-2", ParentNodeID: &clause, Kind: "diem", Path: "dieu-1/khoan-1/diem-b", Ordinal: 1, Text: strings.Repeat("q", 20)},
			}
			recipe := Recipe1()
			recipe.BodyLimit, recipe.MinimumRunes = 30, 1
			file, _, err := Split(nodes, Meta{Key: "vn:vbpl:1"}, recipe)
			if err != nil {
				t.Fatal(err)
			}
			articleCount := 0
			clauseCount := 0
			points := 0
			for _, chunk := range file.Chunks {
				if strings.Contains(chunk.Content, tc.articleLead) {
					articleCount++
					if chunk.NodeID != article || chunk.Content != tc.articleLead {
						t.Errorf("article lead emitted under %q: %q", chunk.NodeID, chunk.Content)
					}
				}
				if strings.HasPrefix(chunk.NodeID, "point-") {
					points++
					if tc.name == "article lead exceeds limit" && !strings.HasPrefix(chunk.Content, clauseLead+"\n") {
						t.Errorf("missing clause lead on %q: %q", chunk.NodeID, chunk.Content)
					}
				}
				if chunk.NodeID == clause && chunk.Content == clauseLead {
					clauseCount++
				}
			}
			if articleCount != 1 || points != 2 {
				t.Fatalf("got %d article leads and %d points: %#v", articleCount, points, file.Chunks)
			}
			if tc.name == "combined leads leave no room" && clauseCount != 1 {
				t.Errorf("got %d standalone clause leads, want 1: %#v", clauseCount, file.Chunks)
			}
		})
	}
}
