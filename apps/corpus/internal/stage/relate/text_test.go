package relate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
)

func TestTextRelations_Emission(t *testing.T) {
	var bundle struct {
		Detail, Diagram json.RawMessage
		Provisions      []chunk.Node
	}
	data, err := os.ReadFile("testdata/text.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &bundle); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, number, gap string
		duplicate         bool
		count             int
	}{
		{name: "linked and deduplicated", count: 1},
		{name: "normalized number", number: "39 / 2016 / tt - nhnn", count: 1},
		{name: "unlinked", number: "40/2016/TT-NHNN", gap: "relation_text_unlinked_target"},
		{name: "ambiguous", duplicate: true, gap: "relation_text_ambiguous_target"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, _, e := relations(bundle.Detail, bundle.Diagram, nil)
			if e != nil {
				t.Fatal(e)
			}
			if tc.number != "" {
				rows[0].TargetNumber = tc.number
			}
			if tc.duplicate {
				other := rows[0]
				other.TargetVBPLID = "43"
				rows = append(rows, other)
			}
			base := len(rows)
			dir := t.TempDir()
			writeTextFixture(t, dir, "provisions.json", bundle.Provisions)
			writeTextFixture(t, dir, "report.json", []stage.Gap{})
			got, report, e := textRelations(dir, rows, map[string]string{"NHNN": "NGÂN HÀNG NHÀ NƯỚC VIỆT NAM"})
			if e != nil {
				t.Fatal(e)
			}
			if len(got) != base+tc.count {
				t.Fatalf("rows=%+v report=%+v", got, report)
			}
			if tc.count == 1 {
				r := got[base]
				if r.Ordinal != base || r.TargetVBPLID != "42" || r.TargetNumber != rows[0].TargetNumber || r.TargetAnchor != "dieu-8/khoan-9" || r.SourceNodeID == nil || *r.SourceNodeID != "article-1" || r.IssuerCode != "NHNN" || r.Kind != "suspends" || r.Method != "text_rule/1" || r.RawCode != "text" || r.RawLabel != "ngưng hiệu lực" || r.StatedIssueDate == nil || *r.StatedIssueDate != "2016-12-30" || r.Evidence == "" {
					t.Fatalf("row=%+v", r)
				}
				if report.Emitted["suspends"] != 1 || len(report.GapDetails) != 0 {
					t.Fatalf("report=%+v", report)
				}
			} else if report.Gaps[tc.gap] != 2 || len(report.GapDetails) != 2 || report.GapDetails[0].Detail != "39/2016/TT-NHNN" || report.GapDetails[0].SourceNodeID != "article-1" {
				t.Fatalf("report=%+v", report)
			}
		})
	}
}

func TestTextRelations_SourceQualityGate(t *testing.T) {
	for _, name := range []string{"tree_headings_only", "structure_uncertain", "table_text", "image_content", "ocr_flagged", "ocr_rejected", "ocr_label_flagged", "needs_ocr"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			// A gated source must not even read the provisions.
			writeTextFixture(t, dir, "report.json", []stage.Gap{{Kind: name}})
			rows, report, err := textRelations(dir, nil, nil)
			if err != nil || len(rows) != 0 || report.Gaps["relation_text_source_incomplete"] != 1 || len(report.GapDetails) != 1 || report.GapDetails[0].Detail != name {
				t.Fatalf("rows=%v report=%+v err=%v", rows, report, err)
			}
		})
	}
}

func TestTextRelations_ArticleScopeAndParserGaps(t *testing.T) {
	dir := t.TempDir()
	writeTextFixture(t, dir, "report.json", []stage.Gap{})
	writeTextFixture(t, dir, "provisions.json", []chunk.Node{
		{NodeID: "clause", Kind: "khoan", Text: `Ngưng hiệu lực khoản 9 Điều 8 của Thông tư 39/2016/TT-NHNN.`},
		{NodeID: "empty", Kind: "dieu", Text: "   "},
		{NodeID: "bad", Kind: "dieu", Text: `Bãi bỏ khoản 1.`},
	})
	rows, report, err := textRelations(dir, nil, map[string]string{"NHNN": "NHNN"})
	if err != nil || len(rows) != 0 || report.Gaps["relation_text_grammar"] != 1 || report.GapDetails[0].SourceNodeID != "bad" {
		t.Fatalf("rows=%v report=%+v err=%v", rows, report, err)
	}
}

func writeTextFixture(t *testing.T, dir, name string, value any) {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, name), b, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRelationReport_ResolveTextGapsAndUnknownTargets(t *testing.T) {
	report := relationReport{Unmapped: []string{"13"}, Text: newTextReport()}
	report.Text.addGap("relation_text_unlinked_target", "40/2016/TT-NHNN", "a")
	rows := []relation{{Method: "text_rule/1", TargetVBPLID: "42"}}
	gaps := report.resolveGaps(rows, map[string]bool{})
	if len(gaps) != 3 || gaps[0].Kind != "unknown_relation_code" || gaps[1] != (stage.Gap{Kind: "unknown_target", Detail: "42"}) || gaps[2] != (stage.Gap{Kind: "relation_text_unlinked_target", Detail: "0:a:40/2016/TT-NHNN"}) {
		t.Fatalf("gaps=%+v", gaps)
	}
	gaps = report.resolveGaps(rows, map[string]bool{"vn:vbpl:42": true})
	if len(gaps) != 2 {
		t.Fatalf("gaps=%+v", gaps)
	}
}

func TestRelationJSON_PreservesDiagramAndNullableTextDate(t *testing.T) {
	diagram := relation{Ordinal: 0, Kind: "repeals", RawCode: "9", Method: "vbpl_diagram", TargetVBPLID: "42"}
	b, err := json.Marshal(diagram)
	want := `{"ordinal":0,"kind":"repeals","raw_code":"9","raw_label":"","method":"vbpl_diagram","evidence":"","target_vbpl_id":"42","target_number":"","source_node_id":null,"target_node_id":null}`
	if err != nil || string(b) != want {
		t.Fatalf("diagram=%s err=%v", b, err)
	}
	diagram.Method = "text_rule/1"
	b, err = json.Marshal(diagram)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(b, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["stated_issue_date"]) != "null" {
		t.Fatalf("text=%s", b)
	}
}

func TestRelationReport_NewShapeOnly(t *testing.T) {
	for _, tc := range []struct {
		name string
		text *textReport
		want string
	}{
		{"primary", newTextReport(), `{"unmapped":[],"text":{"emitted":{},"gaps":{},"gap_details":[]}}`},
		{"secondary", nil, `{"unmapped":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(relationReport{Unmapped: []string{}, Text: tc.text})
			if err != nil || string(b) != tc.want {
				t.Fatalf("report=%s err=%v", b, err)
			}
		})
	}
	var r relationReport
	if err := json.Unmarshal([]byte(`[]`), &r); err == nil {
		t.Fatal("old array report accepted")
	}
}

func TestTextRelations_DeterministicOrder(t *testing.T) {
	dir := t.TempDir()
	writeTextFixture(t, dir, "report.json", []stage.Gap{})
	articles := []chunk.Node{
		{NodeID: "b", Kind: "dieu", Text: "Bãi bỏ Điều 2 của Thông tư 39/2016/TT-NHNN."},
		{NodeID: "a", Kind: "dieu", Text: "Bãi bỏ Điều 1 của Thông tư 39/2016/TT-NHNN."},
	}
	var previous string
	for i := 0; i < 2; i++ {
		writeTextFixture(t, dir, "provisions.json", articles)
		rows, _, err := textRelations(dir, []relation{{Ordinal: 0, TargetVBPLID: "42", TargetNumber: "39/2016/TT-NHNN"}}, map[string]string{"NHNN": "NHNN"})
		if err != nil {
			t.Fatal(err)
		}
		b, err := json.Marshal(rows)
		if err != nil {
			t.Fatal(err)
		}
		if i > 0 && string(b) != previous {
			t.Fatalf("order changed: %s versus %s", b, previous)
		}
		if len(rows) != 3 || rows[1].Ordinal != 1 || rows[2].Ordinal != 2 || *rows[1].SourceNodeID != "a" {
			t.Fatalf("rows=%+v", rows)
		}
		previous = string(b)
		articles[0], articles[1] = articles[1], articles[0]
	}
}

func TestRelationReport_RunGapDetailIsBoundedAndUnique(t *testing.T) {
	long := strings.Repeat("Điều khoản sửa đổi ", 600) // over 10,000 characters
	report := relationReport{Text: newTextReport()}
	report.Text.addGap("relation_text_grammar", long, "dieu-1")
	report.Text.addGap("relation_text_grammar", long, "dieu-1")
	report.Text.addGap("relation_text_unbalanced_quote", long, "dieu-2")
	gaps := report.resolveGaps(nil, map[string]bool{})
	if len(gaps) != 3 {
		t.Fatalf("gaps=%d", len(gaps))
	}
	seen := map[string]bool{}
	for _, gap := range gaps {
		// The workshop gap table indexes detail; a btree entry holds at most 2704 bytes.
		if len(gap.Detail) > 1024 || !utf8.ValidString(gap.Detail) || seen[gap.Kind+gap.Detail] {
			t.Fatalf("detail bytes=%d valid=%v duplicate=%v", len(gap.Detail), utf8.ValidString(gap.Detail), seen[gap.Kind+gap.Detail])
		}
		seen[gap.Kind+gap.Detail] = true
	}
	if !strings.HasPrefix(gaps[0].Detail, "0:dieu-1:Điều khoản") || report.Text.GapDetails[0].Detail != long {
		t.Fatalf("detail=%q", gaps[0].Detail)
	}
}
