package extract

import (
	"strings"
	"testing"
	"unicode/utf8"

	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
)

func TestExtract_AcceptedChildSpansMapToTheirParagraphs(t *testing.T) {
	paragraphs := []string{
		"Điều 1. Nội dung", "Dẫn \"", "1. X\"", "1. X\"", "Kết thúc riêng\"",
		"Dẫn \"", "a) X\"", "a) X\"", "Đoạn kết riêng của điểm a\"",
		"b) " + strings.Repeat("Trách nhiệm bảo vệ tài nguyên. ", 40) + "Kết thúc.",
		"2. " + strings.Repeat("Quyền và nghĩa vụ thực hiện. ", 35) + "Thi hành.",
	}
	body := "<p>" + strings.Repeat("Lời mở đầu ", 40) + "</p><p>" + strings.Join(paragraphs, "</p><p>") + "</p>"
	rows, gaps, record := parseTranscription([]byte(body), "1_content.html", "fixture")
	if len(gaps) != 0 {
		t.Fatal(gaps)
	}
	counts := record.Counts.Structure
	if counts == nil || counts.ArticlesSplit != 1 || counts.Clauses != 2 || counts.Points != 2 || len(counts.Reasons) != 0 {
		t.Fatalf("counts=%+v", counts)
	}
	expected := map[string]int{"dieu-1/khoan-1": 3, "dieu-1/khoan-1/diem-a": 7, "dieu-1/khoan-1/diem-b": 9, "dieu-1/khoan-2": 10}
	offsets := map[string]int{}
	articleID := ""
	var nodes []chunk.Node
	for _, p := range rows {
		nodes = append(nodes, chunk.Node{NodeID: p.NodeID, ParentNodeID: p.ParentNodeID, Kind: p.Kind, Label: p.Label, Heading: p.Heading, Ordinal: p.Ordinal, Path: p.Path, Text: p.Text})
		if p.Path == "dieu-1" {
			articleID = p.NodeID
		}
		if index, ok := expected[p.Path]; ok {
			offsets[p.NodeID] = utf8.RuneCountInString(strings.Join(paragraphs[:index], "\n")) + 1
		}
	}
	if articleID == "" || len(offsets) != 4 {
		t.Fatalf("article=%s child offsets=%v", articleID, offsets)
	}
	file, coverage, err := chunk.Split(nodes, chunk.Meta{Key: "vn:vbpl:1"}, chunk.Recipe2())
	if err != nil {
		t.Fatal(err)
	}
	for _, gap := range coverage.Gaps {
		if gap.Kind == "ambiguous_tree_text" {
			t.Fatal(gap)
		}
	}
	seen := map[string]bool{}
	owners := map[string]bool{}
	for _, c := range file.Chunks {
		owners[c.Anchor] = true
		for _, span := range c.Spans {
			offset, ok := offsets[span.NodeID]
			if !ok {
				continue
			}
			seen[span.NodeID] = true
			for position := span.ContentStart; position < span.ContentEnd; position++ {
				expectedOffset := offset + span.Start + position - span.ContentStart
				mapped := false
				for _, alias := range c.Spans {
					if alias.NodeID == articleID && position >= alias.ContentStart && position < alias.ContentEnd && alias.Start+position-alias.ContentStart == expectedOffset {
						mapped = true
						break
					}
				}
				if !mapped {
					t.Fatalf("%s content offset %d lacks article source offset %d", c.Anchor, position, expectedOffset)
				}
			}
		}
	}
	if len(seen) != 4 {
		t.Fatalf("mapped children=%d want 4", len(seen))
	}
	for _, anchor := range []string{"dieu-1/khoan-1/diem-a", "dieu-1/khoan-1/diem-b", "dieu-1/khoan-2"} {
		if !owners[anchor] {
			t.Errorf("missing chunk owner %s: %v", anchor, owners)
		}
	}
}
