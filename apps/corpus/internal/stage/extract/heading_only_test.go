package extract

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func headingBundle(t *testing.T, tree string, withText bool) string {
	t.Helper()
	dir := t.TempDir()
	files := `[]`
	if withText {
		data := []byte("<p>Điều 1. Phạm vi điều chỉnh</p><p>" + strings.Repeat("nội dung ", 60) + "</p>")
		hash := fmt.Sprintf("%x", sha256.Sum256(data))
		if err := os.Mkdir(filepath.Join(dir, "files"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "files", hash), data, 0600); err != nil {
			t.Fatal(err)
		}
		files = fmt.Sprintf(`[{"name":"1_content.html","sha256":"%s","size":%d}]`, hash, len(data))
	}
	for name, data := range map[string]string{"tree.json": tree, "detail.json": `{"data":{"id":"1"}}`, "files.json": files} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const bareTree = `{"success":true,"data":[{"id":"a","level":"Article","title":"Điều 1. Phạm vi điều chỉnh","content":{"content":"Điều 1. Phạm vi điều chỉnh"},"isLeaf":true}]}`

func TestExtract_HeadingOnlyTreeUsesTranscription(t *testing.T) {
	s, nodes, err := choose(context.Background(), headingBundle(t, bareTree, true), "vn:vbpl:1")
	if err != nil || s.source != "html_transcription" || len(nodes) != 0 || selectionHash("fetch", s.source) != InputHash("fetch", "html_transcription") {
		t.Fatalf("selection=%+v nodes=%v err=%v", s, nodes, err)
	}
	rows, _, record := parseTranscription(s.data, s.name, s.hash)
	if len(rows) == 0 || record == nil || record.TextSource != "html_transcription" {
		t.Fatalf("rows=%v record=%+v", rows, record)
	}
}

func TestExtract_HeadingOnlyTreeWithoutTextKeepsTreeWithGap(t *testing.T) {
	s, nodes, err := choose(context.Background(), headingBundle(t, bareTree, false), "vn:vbpl:1")
	if err != nil || s.source != "source_tree" || s.gap != "tree_headings_only" || len(nodes) != 1 || selectionHash("fetch", s.source) != "fetch" {
		t.Fatalf("selection=%+v nodes=%v err=%v", s, nodes, err)
	}
}

func TestExtract_PartlyBareTreeStaysSourceTree(t *testing.T) {
	tree := `{"success":true,"data":[{"id":"a","level":"Article","title":"Điều 1. Phạm vi","content":{"content":"Điều 1. Phạm vi"}},{"id":"b","level":"Article","title":"Điều 2. Quy định","content":{"content":"Điều 2. Quy định<br>Nội dung"}}]}`
	s, nodes, err := choose(context.Background(), headingBundle(t, tree, true), "vn:vbpl:1")
	if err != nil || s.source != "source_tree" || s.gap != "" || len(nodes) != 2 {
		t.Fatalf("selection=%+v nodes=%v err=%v", s, nodes, err)
	}
}

func TestSelection_HeadingOnlyDefinition(t *testing.T) {
	for _, tc := range []struct {
		name, tree string
		want       bool
	}{
		{"heading", bareTree, true},
		{"whitespace", `{"success":true,"data":[{"id":"a","level":"Article","title":"Điều 1. Phạm vi","content":{"content":"Điều 1.  Phạm vi"}}]}`, true},
		{"nested article", `{"success":true,"data":[{"id":"c","level":"Chapter","title":"Chương I","children":[{"id":"a","level":"Article","title":"Điều 1. Phạm vi","content":{"content":"Điều 1. Phạm vi"}}]}]}`, true},
		{"child", `{"success":true,"data":[{"id":"a","level":"Article","title":"Điều 1. Phạm vi","content":{"content":"Điều 1. Phạm vi"},"children":[{"id":"b","level":"Clause"}]}]}`, false},
		{"body", `{"success":true,"data":[{"id":"a","level":"Article","title":"Điều 1. Phạm vi","content":{"content":"Điều 1. Phạm vi<br>Nội dung"}}]}`, false},
		{"chapter body", `{"success":true,"data":[{"id":"c","level":"Chapter","title":"Chương I","content":{"content":"Chương I<br>Nội dung"},"children":[{"id":"a","level":"Article","title":"Điều 1. Phạm vi","content":{"content":"Điều 1. Phạm vi"}}]}]}`, false},
		{"no article", `{"success":true,"data":[{"id":"a","level":"Chapter","title":"Chương I"}]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, err := choose(context.Background(), headingBundle(t, tc.tree, true), "vn:vbpl:1")
			if err != nil {
				t.Fatal(err)
			}
			if got := s.source == "html_transcription"; got != tc.want {
				t.Fatalf("selection=%+v want heading-only=%v", s, tc.want)
			}
		})
	}
}

func TestSelection_GoldenTreeRemainsSourceTree(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "vbpl", "testdata", "tree-67878.json"))
	if err != nil {
		t.Fatal(err)
	}
	s, _, err := choose(context.Background(), headingBundle(t, string(data), true), "vn:vbpl:1")
	if err != nil || s.source != "source_tree" || s.gap != "" {
		t.Fatalf("selection=%+v err=%v", s, err)
	}
}

func TestExtract_HeadingOnlyTreeRejectsOverDepthTranscription(t *testing.T) {
	dir := headingBundle(t, bareTree, false)
	data := []byte(strings.Repeat("<div>", 70) + strings.Repeat("nội dung ", 60) + strings.Repeat("</div>", 70))
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	if err := os.Mkdir(filepath.Join(dir, "files"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "files", hash), data, 0600); err != nil {
		t.Fatal(err)
	}
	files := fmt.Sprintf(`[{"name":"1_content.html","sha256":"%s","size":%d}]`, hash, len(data))
	if err := os.WriteFile(filepath.Join(dir, "files.json"), []byte(files), 0600); err != nil {
		t.Fatal(err)
	}
	s, nodes, err := choose(context.Background(), dir, "vn:vbpl:1")
	if err != nil || s.source != "source_tree" || s.gap != "tree_headings_only" || len(nodes) != 1 {
		t.Fatalf("selection=%+v nodes=%v err=%v", s, nodes, err)
	}
}
