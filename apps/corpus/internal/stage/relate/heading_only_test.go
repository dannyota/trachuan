package relate

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
)

func TestTextRelations_HeadingOnlyArticles(t *testing.T) {
	const heading = "Bãi bỏ Điều 25 của Thông tư số 09/2020/TT-NHNN"
	for _, tc := range []struct {
		name     string
		gaps     []stage.Gap
		children bool
		want     int
	}{
		{name: "partial tree", gaps: []stage.Gap{{Kind: "tree_headings_partial"}}},
		{name: "heading-only tree", gaps: []stage.Gap{{Kind: "tree_headings_only"}}},
		{name: "complete source", want: 1},
		{name: "article with child", gaps: []stage.Gap{{Kind: "tree_headings_partial"}}, children: true, want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeTextFixture(t, dir, "report.json", tc.gaps)
			nodes := []chunk.Node{{NodeID: "article", Kind: "dieu", Label: "Điều 1", Heading: heading, Text: "Điều 1. " + heading}}
			if tc.children {
				parent := "article"
				nodes = append(nodes, chunk.Node{NodeID: "child", ParentNodeID: &parent, Kind: "khoan", Text: "Nội dung."})
			}
			writeTextFixture(t, dir, "provisions.json", nodes)
			links := []relation{{TargetVBPLID: "42", TargetNumber: "09/2020/TT-NHNN"}}
			rows, report, err := textRelations(dir, links, map[string]string{"NHNN": "NHNN"})
			if err != nil || len(rows) != 1+tc.want || len(report.GapDetails) != 0 {
				t.Fatalf("rows=%+v report=%+v err=%v", rows, report, err)
			}
		})
	}
}
