package relate

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
)

func TestTextRelations_GapsRequireCandidates(t *testing.T) {
	const lead = "Điều 1. Sửa đổi một số điều của Thông tư 19/2018/TT-NHNN như sau:"
	for _, tc := range []struct {
		name, text, gap string
	}{
		{name: "heading", text: "Điều 1. Phạm vi điều chỉnh"},
		{name: "heading before noncandidate list", text: "Điều 1. Phạm vi điều chỉnh\n1. Nội dung áp dụng."},
		{name: "definitions", text: "Điều 3. Giải thích từ ngữ. Các từ ngữ được hiểu như sau:\n1. Nội dung định nghĩa."},
		{name: "unbalanced without candidate", text: "nội dung”"},
		{name: "unbalanced quoted candidate", text: "“Bãi bỏ Điều 5 của Thông tư 19/2018/TT-NHNN."},
		{name: "unquoted amendment", text: lead + "\n1. Sửa đổi khoản 2 Điều 8 như sau:\n2. Bãi bỏ Điều 5.", gap: "relation_text_unquoted_wording"},
		{name: "unbalanced amendment", text: lead + "\n1. Sửa đổi khoản 2 Điều 8 như sau: “nội dung", gap: "relation_text_unbalanced_quote"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeTextFixture(t, dir, "report.json", []stage.Gap{})
			writeTextFixture(t, dir, "provisions.json", []chunk.Node{{NodeID: "article", Kind: "dieu", Text: tc.text}})
			links := []relation{{TargetNumber: "19/2018/TT-NHNN", TargetVBPLID: "42", Method: "vbpl_diagram"}}
			rows, report, err := textRelations(dir, links, map[string]string{"NHNN": "NHNN"})
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 || rows[0] != links[0] || len(report.Emitted) != 0 {
				t.Fatalf("rows=%+v report=%+v; want only diagram row", rows, report)
			}
			if tc.gap == "" {
				if len(report.Gaps) != 0 || len(report.GapDetails) != 0 {
					t.Fatalf("unexpected report gaps: %+v", report)
				}
			} else if len(report.Gaps) != 1 || report.Gaps[tc.gap] != 1 || len(report.GapDetails) != 1 || report.GapDetails[0].SourceNodeID != "article" {
				t.Fatalf("report=%+v; want one %s on article", report, tc.gap)
			}
		})
	}
}
