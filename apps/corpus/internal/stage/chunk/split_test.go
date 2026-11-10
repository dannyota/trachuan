package chunk

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func TestSplit_SmallArticleOneChunk(t *testing.T) {
	nodes := []Node{{NodeID: "a", Kind: "dieu", Path: "dieu-1", Text: "A substantive article with enough text to retain."}}
	file, _, err := Split(nodes, Meta{Key: "vn:vbpl:1"}, Recipe1())
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Chunks) != 1 || file.Chunks[0].NodeID != "a" || file.Chunks[0].Content != nodes[0].Text {
		t.Fatalf("unexpected chunks: %#v", file.Chunks)
	}
}

func TestSplit_EmptyTreeNoTreeGap(t *testing.T) {
	file, coverage, err := Split(nil, Meta{Key: "vn:vbpl:1"}, Recipe1())
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Chunks) != 0 {
		t.Fatalf("got %d chunks for empty tree", len(file.Chunks))
	}
	for _, gap := range coverage.Gaps {
		if gap.Kind == "no_tree" {
			return
		}
	}
	t.Fatal("missing no_tree gap")
}

func TestSplit_InvalidParentIsSourceData(t *testing.T) {
	parent := "missing"
	_, _, err := Split([]Node{{NodeID: "a", ParentNodeID: &parent, Kind: "dieu", Path: "dieu-1", Text: "Text"}}, Meta{Key: "vn:vbpl:1"}, Recipe1())
	if !errors.Is(err, stage.ErrSourceData) {
		t.Fatalf("got error %v, want source data error", err)
	}
}

func TestSplit_OrdinalsRepeatable(t *testing.T) {
	nodes := []Node{{NodeID: "a", Kind: "dieu", Path: "dieu-1", Text: strings.Repeat("A substantive sentence. ", 100)}}
	first, firstCoverage, err := Split(nodes, Meta{Key: "vn:vbpl:1"}, Recipe1())
	if err != nil {
		t.Fatal(err)
	}
	second, secondCoverage, err := Split(nodes, Meta{Key: "vn:vbpl:1"}, Recipe1())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(firstCoverage, secondCoverage) {
		t.Fatal("split output changed between runs")
	}
	for i, chunk := range first.Chunks {
		if chunk.Ordinal != i {
			t.Fatalf("chunk %d has ordinal %d", i, chunk.Ordinal)
		}
	}
}

func TestSplit_ShortAndLabelLeavesOmitted(t *testing.T) {
	nodes := []Node{
		{NodeID: "a", Kind: "dieu", Path: "dieu-1", Label: "Điều 1", Text: "Điều 1."},
		{NodeID: "b", Kind: "dieu", Path: "dieu-2", Text: "Short duty"},
	}
	file, coverage, err := Split(nodes, Meta{Key: "vn:vbpl:1"}, Recipe1())
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Chunks) != 0 {
		t.Fatalf("got %d chunks, want none", len(file.Chunks))
	}
	reasons := make(map[string]bool)
	for _, omission := range coverage.Omissions {
		reasons[omission.Reason] = true
	}
	if !reasons["label_only"] || !reasons["short"] {
		t.Fatalf("missing omission reasons: %v", reasons)
	}
}

func TestSplit_DuplicateKeepsFirstAndRecordsOrdinal(t *testing.T) {
	body := "This substantive provision appears twice in the document."
	nodes := []Node{
		{NodeID: "a", Kind: "dieu", Path: "dieu-1", Text: body},
		{NodeID: "b", Kind: "dieu", Path: "dieu-2", Ordinal: 1, Text: body},
	}
	file, coverage, err := Split(nodes, Meta{Key: "vn:vbpl:1"}, Recipe1())
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Chunks) != 1 || file.Chunks[0].NodeID != "a" {
		t.Fatalf("unexpected chunks: %#v", file.Chunks)
	}
	for _, omission := range coverage.Omissions {
		if omission.NodeID == "b" && omission.Reason == "duplicate" && omission.KeptOrdinal != nil && *omission.KeptOrdinal == 0 {
			return
		}
	}
	t.Fatalf("missing duplicate omission: %#v", coverage.Omissions)
}
