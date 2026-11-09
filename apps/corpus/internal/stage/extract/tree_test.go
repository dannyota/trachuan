package extract

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/traquydinh/apps/corpus/internal/stage"
	"danny.vn/traquydinh/apps/corpus/internal/vbpl"
)

func fixture(t *testing.T, name string) []vbpl.Node {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "vbpl", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Data []vbpl.Node `json:"data"`
	}
	if err = json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.Data
}
func TestExtract_Fixture67878(t *testing.T) {
	rows, _, err := extractTree(context.Background(), fixture(t, "tree-67878.json"))
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, row := range rows {
		counts[row.Kind]++
	}
	if counts["dieu"] != 11 || counts["khoan"] != 34 || counts["diem"] != 17 {
		t.Fatal(counts)
	}
	if rows[0].Label != "Điều 1" || rows[0].Heading != "Phạm vi điều chỉnh và đối tượng áp dụng" || rows[0].Path != "dieu-1" {
		t.Fatal(rows[0])
	}
	found := false
	for _, row := range rows {
		if row.Path == "dieu-2/khoan-1" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing clause path")
	}
}
func TestExtract_EmptyTreeIsGap(t *testing.T) {
	rows, gaps, err := extractTree(context.Background(), fixture(t, "tree-66518.json"))
	if err != nil || len(rows) != 0 || len(gaps) != 1 || gaps[0].Kind != "no_tree" {
		t.Fatalf("%v %v %v", rows, gaps, err)
	}
}
func TestExtract_DuplicatePathsDisambiguated(t *testing.T) {
	nodes := []vbpl.Node{{ID: "1", Level: "Article", Title: "Điều 5", Content: "a"}, {ID: "2", Level: "Article", Title: "Điều 5", Content: "b"}}
	rows, _, err := extractTree(context.Background(), nodes)
	if err != nil || rows[1].Path != "dieu-5~2" {
		t.Fatalf("%v %v", rows, err)
	}
}
func TestExtract_ArticleSequenceGap(t *testing.T) {
	nodes := []vbpl.Node{{ID: "1", Level: "Article", Title: "Điều 1", Content: "a"}, {ID: "2", Level: "Article", Title: "Điều 2", Content: "a"}, {ID: "4", Level: "Article", Title: "Điều 4", Content: "a"}}
	_, gaps, err := extractTree(context.Background(), nodes)
	if err != nil || !hasGap(gaps, "article_sequence", "3") {
		t.Fatalf("%v %v", gaps, err)
	}
}
func TestExtract_HugeArticleNumberIsOneGap(t *testing.T) {
	nodes := []vbpl.Node{{ID: "1", Level: "Article", Title: "Điều 999999999999", Content: "a"}}
	_, gaps, err := extractTree(context.Background(), nodes)
	if err != nil || len(gaps) != 1 || !hasGap(gaps, "article_number_out_of_range", "999999999999") {
		t.Fatalf("%v %v", gaps, err)
	}
}

func TestExtract_MissingArticlesAsRanges(t *testing.T) {
	nodes := []vbpl.Node{{ID: "1", Level: "Article", Title: "Điều 1", Content: "a"}, {ID: "8", Level: "Article", Title: "Điều 8", Content: "a"}}
	_, gaps, err := extractTree(context.Background(), nodes)
	if err != nil || len(gaps) != 1 || !hasGap(gaps, "article_sequence", "2-7") {
		t.Fatalf("%v %v", gaps, err)
	}
}
func TestExtract_UnknownKindIsOther(t *testing.T) {
	rows, gaps, err := extractTree(context.Background(), []vbpl.Node{{ID: "x", Level: "unknown", Content: "a"}})
	if err != nil || rows[0].Kind != "other" || !hasGap(gaps, "unknown_node_kind", "x") {
		t.Fatalf("%v %v %v", rows, gaps, err)
	}
}
func TestExtract_EmptyArticleTextIsGap(t *testing.T) {
	_, gaps, err := extractTree(context.Background(), []vbpl.Node{{ID: "x", Level: "Article", Title: "Điều 1"}})
	if err != nil || !hasGap(gaps, "empty_text", "x") {
		t.Fatalf("%v %v", gaps, err)
	}
}
func TestExtract_TreeTooLargeIsGap(t *testing.T) {
	n := vbpl.Node{ID: "x"}
	for i := 0; i < 32; i++ {
		n = vbpl.Node{ID: "x", Children: []vbpl.Node{n}}
	}
	for _, nodes := range [][]vbpl.Node{{n}, make([]vbpl.Node, 50001)} {
		rows, gaps, err := extractTree(context.Background(), nodes)
		if err != nil || len(rows) != 0 || !hasGap(gaps, "tree_too_large", "") {
			t.Fatalf("%d %v %v", len(rows), gaps, err)
		}
	}
}
func TestText_ConvertsBreaksAndEntities(t *testing.T) {
	if got := textFromHTML("<p>A &amp; B<br/>C</p>"); got != "A & B\nC" {
		t.Fatal(got)
	}
}
func TestText_NormalizesToNFC(t *testing.T) {
	if got := textFromHTML("e\u0301"); strings.Compare(got, "é") != 0 {
		t.Fatal(got)
	}
}

func hasGap(gaps []stage.Gap, kind, detail string) bool {
	for _, gap := range gaps {
		if gap.Kind == kind && gap.Detail == detail {
			return true
		}
	}
	return false
}

func TestExtract_Labels(t *testing.T) {
	cases := []struct{ kind, title, label, heading, path string }{
		{"phan", "Phần thứ nhất", "Phần thứ nhất", "", "phan-1"},
		{"chuong", "Chương II\nQuy định chung", "Chương II", "Quy định chung", "chuong-ii"},
		{"muc", "Mục 1. Phạm vi", "Mục 1", "Phạm vi", "muc-1"},
		{"khoan", "Khoản 1", "Khoản 1", "", "khoan-1"},
		{"diem", "Điểm a", "Điểm a", "", "diem-a"},
	}
	for _, tc := range cases {
		t.Run(tc.title, func(t *testing.T) {
			label, heading, path, _ := titleParts(tc.kind, tc.title, 3)
			if label != tc.label || heading != tc.heading || path != tc.path {
				t.Fatalf("%q %q %q", label, heading, path)
			}
		})
	}
}
