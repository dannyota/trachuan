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
	for _, name := range []string{"tree_headings_only", "tree_headings_partial", "structure_uncertain", "table_text", "image_content", "ocr_flagged", "ocr_rejected", "ocr_label_flagged", "needs_ocr"} {
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

func TestGapDetail_BoundsLongIDsAndFourByteRunes(t *testing.T) {
	node := strings.Repeat("n", 2000)
	text := strings.Repeat("\U0001F600", 5000) // four-byte runes
	got := runGapDetail(7, textGap{Detail: text, SourceNodeID: node})
	if len(got) > 1024 || !utf8.ValidString(got) || !strings.HasPrefix(got, "7:nnn") {
		t.Fatalf("bytes=%d valid=%v", len(got), utf8.ValidString(got))
	}
	a, b := strings.Repeat("Đ", 400)+"a", strings.Repeat("Đ", 400)+"b"
	if x, y := boundedDetail(a, gapDetailBytes), boundedDetail(b, gapDetailBytes); len(x) > gapDetailBytes || !utf8.ValidString(x) || x == y {
		t.Fatalf("x=%d bytes valid=%v distinct=%v", len(x), utf8.ValidString(x), x != y)
	}
	if boundedDetail("12566", gapDetailBytes) != "12566" {
		t.Fatal("short detail changed")
	}
	long := strings.Repeat("9", 3000)
	gaps := resolveGaps([]relation{{TargetVBPLID: long}}, []string{long}, map[string]bool{})
	for _, gap := range gaps {
		if len(gap.Detail) > gapDetailBytes {
			t.Fatalf("%s detail bytes=%d", gap.Kind, len(gap.Detail))
		}
	}
}

func TestTextRelations_ScopedSubtrees(t *testing.T) {
	const lead = "Điều 1. Sửa đổi, bổ sung một số điều của Thông tư 19/2018/TT-NHNN như sau:"
	const item = "1. Bãi bỏ khoản 2 Điều 5."
	parent := "article"
	clause := "clause"
	for _, tc := range []struct {
		name, text, gap string
		children        []chunk.Node
		anchors         []string
	}{
		{name: "text contains descendants", text: lead + "\n" + item, children: []chunk.Node{{NodeID: clause, ParentNodeID: &parent, Kind: "khoan", Text: item}}, anchors: []string{"dieu-5/khoan-2"}},
		{name: "heading and clause nodes in ordinal order", text: lead, children: []chunk.Node{
			{NodeID: "later", ParentNodeID: &parent, Kind: "khoan", Ordinal: 2, Text: "2. Bãi bỏ Điều 6."},
			{NodeID: clause, ParentNodeID: &parent, Kind: "khoan", Ordinal: 1, Text: item},
		}, anchors: []string{"dieu-5/khoan-2", "dieu-6"}},
		{name: "quote spans point nodes", text: lead, children: []chunk.Node{
			{NodeID: clause, ParentNodeID: &parent, Kind: "khoan", Text: "1. Sửa đổi khoản 2 Điều 5 như sau:"},
			{NodeID: "p1", ParentNodeID: &clause, Kind: "diem", Ordinal: 1, Text: "“a) Nội dung."},
			{NodeID: "p2", ParentNodeID: &clause, Kind: "diem", Ordinal: 2, Text: "b) Bãi bỏ Điều 9.”"},
		}, anchors: []string{"dieu-5/khoan-2"}},
		{name: "partial containment", text: lead + "\n" + item, children: []chunk.Node{
			{NodeID: clause, ParentNodeID: &parent, Kind: "khoan", Ordinal: 1, Text: item},
			{NodeID: "later", ParentNodeID: &parent, Kind: "khoan", Ordinal: 2, Text: "2. Bãi bỏ Điều 6."},
		}, gap: "relation_text_source_order"},
		{name: "point beside clause", text: lead, children: []chunk.Node{
			{NodeID: clause, ParentNodeID: &parent, Kind: "khoan", Text: item},
			{NodeID: "p", ParentNodeID: &parent, Kind: "diem", Text: "“nội dung”"},
		}, gap: "relation_text_source_order"},
		{name: "other beside clause with quotation", text: lead, children: []chunk.Node{
			{NodeID: clause, ParentNodeID: &parent, Kind: "khoan", Text: "1. Sửa đổi Điều 5 như sau:"},
			{NodeID: "other", ParentNodeID: &parent, Kind: "other", Text: "“nội dung”"},
		}, gap: "relation_text_source_order"},
		{name: "contained quote stays within one read unit", text: lead + "\n1. Sửa đổi Điều 5 như sau: “nội dung\n2. Bãi bỏ Điều 6.”", children: []chunk.Node{
			{NodeID: clause, ParentNodeID: &parent, Kind: "khoan", Ordinal: 1, Text: "1. Sửa đổi Điều 5 như sau: “nội dung"},
			{NodeID: "later", ParentNodeID: &parent, Kind: "khoan", Ordinal: 2, Text: "2. Bãi bỏ Điều 6.”"},
		}, anchors: []string{"dieu-5"}},
		{name: "quote crosses clauses", text: lead, children: []chunk.Node{
			{NodeID: clause, ParentNodeID: &parent, Kind: "khoan", Ordinal: 1, Text: "1. Sửa đổi Điều 5 như sau: “nội dung"},
			{NodeID: "later", ParentNodeID: &parent, Kind: "khoan", Ordinal: 2, Text: "2. Bãi bỏ Điều 6.”"},
		}, gap: "relation_text_unbalanced_quote"},
		{name: "nested point with document", text: lead, children: []chunk.Node{
			{NodeID: clause, ParentNodeID: &parent, Kind: "khoan", Text: item},
			{NodeID: "p", ParentNodeID: &clause, Kind: "diem", Text: "a) Bãi bỏ Điều 9 của Thông tư 19/2018/TT-NHNN."},
		}, anchors: []string{"dieu-5/khoan-2"}, gap: "relation_text_nested_scope"},
		{name: "numbered second sentence in clause", text: lead, children: []chunk.Node{
			{NodeID: clause, ParentNodeID: &parent, Kind: "khoan", Text: item + "\n2. Bãi bỏ Điều 6."},
		}, anchors: []string{"dieu-5/khoan-2"}, gap: "relation_text_nested_scope"},
		{name: "numbered point with own document", text: lead, children: []chunk.Node{
			{NodeID: clause, ParentNodeID: &parent, Kind: "khoan", Text: item},
			{NodeID: "point", ParentNodeID: &clause, Kind: "diem", Text: "2. Bãi bỏ Điều 6 của Thông tư 19/2018/TT-NHNN."},
		}, anchors: []string{"dieu-5/khoan-2"}, gap: "relation_text_nested_scope"},
		{name: "unscoped contained quote stays within one read unit", text: "Điều 1. Bãi bỏ Điều 5 của Thông tư 19/2018/TT-NHNN.\n1. Nội dung “một\n2. Nội dung hai”", children: []chunk.Node{
			{NodeID: clause, ParentNodeID: &parent, Kind: "khoan", Ordinal: 1, Text: "1. Nội dung “một"},
			{NodeID: "later", ParentNodeID: &parent, Kind: "khoan", Ordinal: 2, Text: "2. Nội dung hai”"},
		}, anchors: []string{"dieu-5"}},
		{name: "contained clause occurs earlier in quotation", text: lead + "\n1. Sửa đổi Điều 5 như sau: “2. Bãi bỏ Điều 6.”\n2. Bãi bỏ Điều 6.", children: []chunk.Node{
			{NodeID: "later", ParentNodeID: &parent, Kind: "khoan", Ordinal: 2, Text: "2. Bãi bỏ Điều 6."},
			{NodeID: clause, ParentNodeID: &parent, Kind: "khoan", Ordinal: 1, Text: "1. Sửa đổi Điều 5 như sau: “2. Bãi bỏ Điều 6.”"},
		}, anchors: []string{"dieu-5", "dieu-6"}},
		{name: "contained text ignores descendant ordinal order", text: lead + "\n" + item + "\n2. Bãi bỏ Điều 6.", children: []chunk.Node{
			{NodeID: clause, ParentNodeID: &parent, Kind: "khoan", Ordinal: 1, Text: "2. Bãi bỏ Điều 6."},
			{NodeID: "later", ParentNodeID: &parent, Kind: "khoan", Ordinal: 2, Text: item},
		}, anchors: []string{"dieu-5/khoan-2", "dieu-6"}},
		{name: "contained text reads duplicate descendants once", text: lead + "\n" + item, children: []chunk.Node{
			{NodeID: clause, ParentNodeID: &parent, Kind: "khoan", Ordinal: 1, Text: item},
			{NodeID: "later", ParentNodeID: &parent, Kind: "khoan", Ordinal: 2, Text: item},
		}, anchors: []string{"dieu-5/khoan-2"}},
		{name: "unscoped text ignores descendant ordinal order", text: "Điều 1. Bãi bỏ Điều 5 của Thông tư 19/2018/TT-NHNN.\n1. Thực hiện.\n2. Theo dõi.", children: []chunk.Node{
			{NodeID: clause, ParentNodeID: &parent, Kind: "khoan", Ordinal: 1, Text: "2. Theo dõi."},
			{NodeID: "later", ParentNodeID: &parent, Kind: "khoan", Ordinal: 2, Text: "1. Thực hiện."},
		}, anchors: []string{"dieu-5"}},
		{name: "unscoped ignores separate clause", text: "Điều 1. Bãi bỏ Điều 5 của Thông tư 19/2018/TT-NHNN.", children: []chunk.Node{{NodeID: clause, ParentNodeID: &parent, Kind: "khoan", Text: "Bãi bỏ Điều 9."}}, anchors: []string{"dieu-5"}},
		{name: "unscoped ignores children", text: "Điều 1. Bãi bỏ Điều 5 của Thông tư 19/2018/TT-NHNN.", children: []chunk.Node{{NodeID: clause, ParentNodeID: &parent, Kind: "other", Text: "Bãi bỏ Điều 9."}}, anchors: []string{"dieu-5"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeTextFixture(t, dir, "report.json", []stage.Gap{})
			nodes := append([]chunk.Node{{NodeID: parent, Kind: "dieu", Text: tc.text}}, tc.children...)
			writeTextFixture(t, dir, "provisions.json", nodes)
			rows, report, err := textRelations(dir, []relation{{TargetNumber: "19/2018/TT-NHNN", TargetVBPLID: "42"}}, map[string]string{"NHNN": "NHNN"})
			if err != nil {
				t.Fatal(err)
			}
			var anchors []string
			for _, row := range rows[1:] {
				anchors = append(anchors, row.TargetAnchor)
				if row.SourceNodeID == nil || *row.SourceNodeID != parent {
					t.Errorf("source=%v", row.SourceNodeID)
				}
				if strings.HasPrefix(tc.text, lead) && !strings.HasPrefix(row.Evidence, lead+"\n") {
					t.Errorf("evidence=%q", row.Evidence)
				}
			}
			if strings.Join(anchors, ",") != strings.Join(tc.anchors, ",") || (tc.gap == "" && len(report.GapDetails) != 0) || (tc.gap != "" && report.Gaps[tc.gap] != 1) {
				t.Fatalf("anchors=%v report=%+v; want %v gap=%s", anchors, report, tc.anchors, tc.gap)
			}
		})
	}
}

func TestTextRelations_RejectedScope(t *testing.T) {
	for _, tc := range []struct {
		name string
		rows []relation
		gap  string
	}{
		{name: "unlinked", gap: "relation_text_unlinked_target"},
		{name: "ambiguous", rows: []relation{{TargetNumber: "19/2018/TT-NHNN", TargetVBPLID: "42"}, {TargetNumber: "19/2018/TT-NHNN", TargetVBPLID: "43"}}, gap: "relation_text_ambiguous_target"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeTextFixture(t, dir, "report.json", []stage.Gap{})
			writeTextFixture(t, dir, "provisions.json", []chunk.Node{{NodeID: "article", Kind: "dieu", Text: "Điều 1. Sửa đổi một số điều của Thông tư 19/2018/TT-NHNN\n1. Bãi bỏ Điều 5.\n2. Bãi bỏ Điều 6."}})
			rows, report, err := textRelations(dir, tc.rows, map[string]string{"NHNN": "NHNN"})
			if err != nil || len(rows) != len(tc.rows) || report.Gaps[tc.gap] != 1 || report.Gaps["relation_text_scope_rejected"] != 2 {
				t.Fatalf("rows=%+v report=%+v err=%v", rows, report, err)
			}
			gaps := (relationReport{Text: report}).resolveGaps(nil, nil)
			if len(gaps) != 3 || !strings.HasPrefix(gaps[1].Detail, "1:article:") {
				t.Fatalf("run gaps=%+v", gaps)
			}
		})
	}
}

func TestTextRelations_QuoteErrors(t *testing.T) {
	for _, text := range []string{"Bãi bỏ Điều 5 của Thông tư 19/2018/TT-NHNN. “nội dung»", "nội dung”. Bãi bỏ Điều 5 của Thông tư 19/2018/TT-NHNN."} {
		dir := t.TempDir()
		writeTextFixture(t, dir, "report.json", []stage.Gap{})
		writeTextFixture(t, dir, "provisions.json", []chunk.Node{{NodeID: "article", Kind: "dieu", Text: text}})
		rows, report, err := textRelations(dir, nil, map[string]string{"NHNN": "NHNN"})
		if err != nil || len(rows) != 0 || report.Gaps["relation_text_unbalanced_quote"] != 1 {
			t.Fatalf("rows=%+v report=%+v err=%v", rows, report, err)
		}
	}
}

func TestTextRelations_SemicolonContinuationUsesOwnDocument(t *testing.T) {
	const text = "Điều 1. Sửa đổi một số điều của Thông tư 19/2018/TT-NHNN như sau:\n1. Bãi bỏ Điều 5;\nĐiều 6 của Thông tư 39/2016/TT-NHNN."
	const evidence = "1. Bãi bỏ Điều 5; Điều 6 của Thông tư 39/2016/TT-NHNN."
	dir := t.TempDir()
	writeTextFixture(t, dir, "report.json", []stage.Gap{})
	writeTextFixture(t, dir, "provisions.json", []chunk.Node{{NodeID: "article", Kind: "dieu", Text: text}})
	links := []relation{
		{TargetNumber: "19/2018/TT-NHNN", TargetVBPLID: "19"},
		{TargetNumber: "39/2016/TT-NHNN", TargetVBPLID: "39"},
	}
	rows, report, err := textRelations(dir, links, map[string]string{"NHNN": "NHNN"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 || len(report.GapDetails) != 0 || report.Emitted["repeals"] != 2 {
		t.Fatalf("rows=%+v report=%+v", rows, report)
	}
	for i, anchor := range []string{"dieu-5", "dieu-6"} {
		row := rows[len(links)+i]
		if row.TargetVBPLID != "39" || row.TargetNumber != "39/2016/TT-NHNN" || row.TargetAnchor != anchor || row.Evidence != evidence {
			t.Errorf("row=%+v; want anchor=%s on 39/2016/TT-NHNN", row, anchor)
		}
	}
}
