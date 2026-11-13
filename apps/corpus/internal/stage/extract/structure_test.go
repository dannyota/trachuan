package extract

import (
	"fmt"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/stage/relate/textrule"
)

func TestExtract_ChildrenParentsAndChunkOwners(t *testing.T) {
	body := "<p>Chương I</p><p>Điều 1. Nội dung</p><p>1. " + strings.Repeat("Trách nhiệm quản lý nguồn nước. ", 50) + "</p><p>2. Thực hiện</p><p>a) " + strings.Repeat("Bảo vệ tài nguyên. ", 50) + "</p><p>b) " + strings.Repeat("Thanh tra nguồn nước. ", 50) + "</p>"
	rows, gaps, record := parseTranscription([]byte(body), "1_content.html", "fixture")
	if len(gaps) != 0 {
		t.Fatal(gaps)
	}
	if len(rows) != 6 {
		t.Fatalf("provisions=%d want 6", len(rows))
	}
	for i, want := range []struct {
		path    string
		parent  int
		ordinal int
	}{
		{"chuong-i", -1, 1}, {"dieu-1", 0, 2}, {"dieu-1/khoan-1", 1, 1}, {"dieu-1/khoan-2", 1, 2}, {"dieu-1/khoan-2/diem-a", 3, 1}, {"dieu-1/khoan-2/diem-b", 3, 2},
	} {
		row := rows[i]
		if row.Path != want.path || row.Ordinal != want.ordinal {
			t.Errorf("row %d: %+v", i, row)
		}
		if want.parent >= 0 && (row.ParentNodeID == nil || *row.ParentNodeID != rows[want.parent].NodeID) {
			t.Errorf("row %d parent=%v want %s", i, row.ParentNodeID, rows[want.parent].NodeID)
		}
	}
	if record.Counts.Structure == nil || record.Counts.Structure.ArticlesSplit != 1 || record.Counts.Structure.Clauses != 2 || record.Counts.Structure.Points != 2 {
		t.Fatalf("counts=%+v", record.Counts)
	}
	nodes := make([]chunk.Node, len(rows))
	for i, p := range rows {
		nodes[i] = chunk.Node{NodeID: p.NodeID, ParentNodeID: p.ParentNodeID, Kind: p.Kind, Label: p.Label, Heading: p.Heading, Ordinal: p.Ordinal, Path: p.Path, Text: p.Text}
	}
	recipe := chunk.Recipe1()
	file, coverage, err := chunk.Split(nodes, chunk.Meta{Key: "vn:vbpl:1"}, recipe)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range coverage.Gaps {
		if g.Kind == "ambiguous_tree_text" {
			t.Fatal(g)
		}
	}
	owners := map[string]bool{}
	for _, c := range file.Chunks {
		owners[c.Anchor] = true
	}
	for _, path := range []string{"dieu-1/khoan-1", "dieu-1/khoan-2/diem-a", "dieu-1/khoan-2/diem-b"} {
		if !owners[path] {
			t.Errorf("missing chunk owner %s; owners=%v", path, owners)
		}
	}
}

func TestExtract_RelationAnchorEquality(t *testing.T) {
	for _, style := range []struct{ clause, point string }{{".", ")"}, {"-", "-"}, {".", "."}} {
		var text strings.Builder
		text.WriteString("<p>Điều 1. Nội dung</p>")
		for _, clause := range []string{"1", "2", "3"} {
			fmt.Fprintf(&text, "<p>%s%s Nội dung</p>", clause, style.clause)
			for _, point := range []string{"a", "b", "c", "d", "đ", "e", "g", "h", "i", "k", "l", "m", "n", "o", "p", "q", "r", "s", "t", "u", "v", "x", "y"} {
				fmt.Fprintf(&text, "<p>%s%s Nội dung %s</p>", point, style.point, point)
			}
		}
		rows, gaps, _ := parseTranscription([]byte(text.String()), "1_content.html", "fixture")
		if len(gaps) != 0 || len(rows) < 2 {
			t.Fatalf("rows=%d gaps=%v", len(rows), gaps)
		}
		for _, p := range rows {
			if p.Kind != "khoan" && p.Kind != "diem" {
				continue
			}
			locator := strings.ToLower(p.Label)
			if p.Kind == "diem" {
				parts := strings.Split(p.Path, "/")
				locator += " khoản " + strings.TrimPrefix(parts[1], "khoan-")
			}
			matches, rejected := textrule.Parse("Bãi bỏ "+locator+" Điều 1 của Thông tư 39/2016/TT-NHNN.", map[string]string{"NHNN": "NGÂN HÀNG NHÀ NƯỚC VIỆT NAM"})
			if len(matches) != 1 || len(rejected) != 0 || len(matches[0].Targets) != 1 || matches[0].Targets[0].Anchor != p.Path {
				t.Errorf("label %s path=%s matches=%+v rejected=%+v", locator, p.Path, matches, rejected)
			}
			_, _, sourceAnchor, _ := titleParts(p.Kind, p.Label, 1)
			if sourceAnchor != strings.Split(p.Path, "/")[strings.Count(p.Path, "/")] {
				t.Errorf("source anchor %s != %s", sourceAnchor, p.Path)
			}
		}
	}
}

func TestExtract_DirectPointAnchors(t *testing.T) {
	data := []byte("<p>Điều 1. Một</p><p>" + strings.Repeat("Nội dung chung. ", 30) + "</p><p>a) A</p><p>b) B</p><p>c) C</p><p>d) D</p><p>đ) Đ</p>")
	rows, gaps, _ := parseTranscription(data, "1_content.html", "fixture")
	if len(gaps) != 0 || len(rows) != 6 {
		t.Fatalf("rows=%+v gaps=%v", rows, gaps)
	}
	for _, p := range rows {
		if p.Kind != "diem" {
			continue
		}
		matches, rejected := textrule.Parse("Bãi bỏ "+p.Label+" Điều 1 của Thông tư 39/2016/TT-NHNN.", map[string]string{"NHNN": "NGÂN HÀNG NHÀ NƯỚC VIỆT NAM"})
		if len(matches) != 1 || len(rejected) != 0 || matches[0].Targets[0].Anchor != p.Path {
			t.Errorf("%s matches=%+v gaps=%+v", p.Path, matches, rejected)
		}
	}
}

func TestExtract_InsertedAnchorsMatchRelations(t *testing.T) {
	data := []byte("<p>" + strings.Repeat("Lời mở đầu. ", 30) + "</p><p>Điều 1. Nội dung</p><p>1. Một</p><p>2. Hai</p><p>a) A</p><p>a1) A1</p><p>b) B</p><p>2a. Thêm</p><p>2b. Thêm nữa</p><p>2c. Thêm nữa</p><p>2d. Thêm nữa</p><p>2đ. Thêm nữa</p><p>3. Ba</p><p>Điều 1a. Bổ sung</p><p>a) A</p><p>a1) A1</p><p>b) B</p>")
	rows, gaps, _ := parseTranscription(data, "1_content.html", "fixture")
	if len(gaps) != 0 {
		t.Fatal(gaps)
	}
	paths := map[string]provision{}
	for _, p := range rows {
		paths[p.Path] = p
	}
	for _, tc := range []struct{ path, label, locator string }{
		{"dieu-1/khoan-2a", "Khoản 2a", "khoản 2a Điều 1"},
		{"dieu-1/khoan-2đ", "Khoản 2đ", "khoản 2đ Điều 1"},
		{"dieu-1/khoan-2/diem-a1", "Điểm a1", "điểm a1 khoản 2 Điều 1"},
		{"dieu-1a/diem-a1", "Điểm a1", "điểm a1 Điều 1a"},
		{"dieu-1a/diem-a", "Điểm a", "điểm a Điều 1a"},
	} {
		p, ok := paths[tc.path]
		if !ok || p.Label != tc.label {
			t.Errorf("extract %s: %+v", tc.path, p)
		}
		matches, rejected := textrule.Parse("Bãi bỏ "+tc.locator+" của Thông tư 39/2016/TT-NHNN.", map[string]string{"NHNN": "NGÂN HÀNG NHÀ NƯỚC VIỆT NAM"})
		if len(matches) != 1 || len(rejected) != 0 || len(matches[0].Targets) != 1 || matches[0].Targets[0].Anchor != p.Path {
			t.Errorf("%s matches=%+v gaps=%+v", tc.path, matches, rejected)
		}
	}
}

func TestExtract_SourceTreeRelationLabelForms(t *testing.T) {
	// Source trees also retain labels outside the parsed-text sequence grammar.
	for _, tc := range []struct{ kind, token, segment string }{
		{"khoan", "0", "khoan-0"}, {"khoan", "01", "khoan-01"}, {"khoan", "1234", "khoan-1234"},
		{"diem", "A", "diem-a"}, {"diem", "Đ", "diem-đ"}, {"diem", "I", "diem-i"}, {"diem", "f", "diem-f"}, {"diem", "Z", "diem-z"}, {"diem", "é", "diem-é"},
	} {
		label := map[string]string{"khoan": "Khoản ", "diem": "Điểm "}[tc.kind] + tc.token
		_, _, segment, _ := titleParts(tc.kind, label, 1)
		if segment != tc.segment {
			t.Errorf("extract %s: %s want %s", label, segment, tc.segment)
		}
		matches, gaps := textrule.Parse("Bãi bỏ "+label+" Điều 5 của Thông tư 39/2016/TT-NHNN.", map[string]string{"NHNN": "NGÂN HÀNG NHÀ NƯỚC VIỆT NAM"})
		if len(matches) != 1 || len(gaps) != 0 || len(matches[0].Targets) != 1 || matches[0].Targets[0].Anchor != "dieu-5/"+segment {
			t.Errorf("%s matches=%+v gaps=%+v", label, matches, gaps)
		}
	}
}
