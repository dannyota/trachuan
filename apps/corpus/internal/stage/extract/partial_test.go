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

func transcriptionBundle(t *testing.T, tree, transcription string) string {
	t.Helper()
	dir := headingBundle(t, tree, false)
	data := []byte(transcription)
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
	return dir
}

const partialTree = `{"success":true,"data":[{"id":"a","level":"Article","title":"Điều 1. Phạm vi","content":{"content":"Điều 1. Phạm vi"}},{"id":"b","level":"Article","title":"Điều 2. Quy định","content":{"content":"Điều 2. Quy định<br>Thời hạn 30 ngày làm việc."}}]}`

func partialHTML() string {
	return "<p>Điều 1. Phạm vi</p><p>" + strings.Repeat("Phạm vi điều chỉnh. ", 25) + "</p><p>Điều 2. Quy định</p><p>Thời hạn 30 ngày làm việc.</p>"
}

func TestSelection_PartialTreeAgreement(t *testing.T) {
	for _, tc := range []struct{ name, html, source, gap, reason string }{
		{"matching", partialHTML(), "html_transcription", "", ""},
		{"rejected", strings.ReplaceAll(partialHTML(), "Điều 2.", "Điều 3."), "source_tree", "tree_headings_partial", "rule 1"},
		{"anchors", partialHTML() + "<p>Điều 3. Thi hành</p><p>Nội dung</p>", "source_tree", "tree_headings_partial", "rule 2"},
		{"number", strings.ReplaceAll(partialHTML(), "30 ngày", "31 ngày"), "source_tree", "tree_headings_partial", "rule 3"},
		{"word", strings.ReplaceAll(partialHTML(), "ngày làm việc", "ngày việc"), "source_tree", "tree_headings_partial", "rule 3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selected, nodes, err := choose(context.Background(), transcriptionBundle(t, partialTree, tc.html), "vn:vbpl:1")
			if err != nil || selected.source != tc.source || selected.gap != tc.gap || !strings.Contains(selected.reason, tc.reason) {
				t.Fatalf("selection=%+v nodes=%d err=%v", selected, len(nodes), err)
			}
		})
	}
}
