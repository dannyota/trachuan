package extract

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
)

func TestExtract_IgnoresOriginAndBackupVariants(t *testing.T) {
	files := []fetch.DocumentFile{{Name: "1_content_origin.html", SHA256: "a"}, {Name: "1_content_20200101.html", SHA256: "b"}, {Name: "1_body_content.html", SHA256: "c"}}
	s, err := selectFiles(t.TempDir(), "1", files, absentOCR{})
	if err != nil || s.source != "none" || s.gap != "no_text_source" {
		t.Fatalf("%+v %v", s, err)
	}
}
func TestExtract_NoSourceGaps(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []fetch.DocumentFile
		gap   string
	}{
		{"empty", nil, "no_text_source"}, {"pdf", []fetch.DocumentFile{{Name: "a.pdf", SHA256: "a"}, {Name: "b.PDF", SHA256: "a"}}, "needs_ocr"},
		{"ambiguous", []fetch.DocumentFile{{Name: "a.pdf", SHA256: "a"}, {Name: "b.pdf", SHA256: "b"}}, "text_source_ambiguous"},
		{"unsupported", []fetch.DocumentFile{{Name: "a.doc"}, {Name: "b.rar"}}, "file_format_unsupported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := selectFiles(t.TempDir(), "1", tc.files, absentOCR{})
			if err != nil || s.gap != tc.gap {
				t.Fatalf("%+v %v", s, err)
			}
		})
	}
}
func TestExtract_InputHashFreezesSelection(t *testing.T) {
	if selectionHash("fetch", "source_tree") != "fetch" || selectionHash("fetch", "none") == selectionHash("fetch", "html_transcription") || selectionHash("fetch", "none") == selectionHash("other", "none") {
		t.Fatal("selection not bound")
	}
}
func TestExtract_HTMLTranscriptionBecomesProvisions(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "textparse", "testdata", "second.html"))
	if err != nil {
		t.Fatal(err)
	}
	rows, gaps, record := parseTranscription(data, "1_content.html", "hash")
	if len(rows) == 0 || record == nil {
		t.Fatalf("rows=%v gaps=%v", rows, gaps)
	}
	found := false
	for _, row := range rows {
		if row.Path == "kem-theo-1/dieu-1" && strings.HasPrefix(row.NodeID, "tx:") {
			found = true
		}
	}
	if !found {
		t.Fatalf("rows=%+v", rows)
	}
}
func TestExtract_LimitErrorIsGap(t *testing.T) {
	rows, gaps, _ := parseTranscription([]byte(strings.Repeat("<div>", 65)+strings.Repeat("x", 300)), "1_content.html", "hash")
	if len(rows) != 0 || !hasGap(gaps, "no_tree", "") || len(gaps) < 2 {
		t.Fatalf("%v %v", rows, gaps)
	}
}

func TestExtract_GeneratedStructureParents(t *testing.T) {
	data := []byte("<p>Điều 1. Mở đầu</p><p>" + strings.Repeat("nội dung ", 40) + "</p><p>QUY CHẾ ĐI KÈM</p><p>Chương I</p><p>QUY ĐỊNH CHUNG</p><p>Điều 1. Phạm vi</p><p>" + strings.Repeat("quy định ", 40) + "</p>")
	rows, _, _ := parseTranscription(data, "1_content.html", "hash")
	for _, row := range rows {
		if row.Path == "kem-theo-1/dieu-1" {
			if row.ParentNodeID == nil {
				t.Fatal("missing chapter parent")
			}
			return
		}
	}
	t.Fatalf("missing article: %+v", rows)
}

func TestExtract_RepeatedNeutralAnchors(t *testing.T) {
	data := []byte("<p>" + strings.Repeat("mở đầu ", 50) + "</p><p>Điều 1. Quy định chung</p><p>" + strings.Repeat("nội dung ", 40) + "</p><p>PHỤ LỤC</p><p>" + strings.Repeat("phụ lục ", 40) + "</p>")
	rows, _, _ := parseTranscription(data, "1_content.html", "hash")
	paths := map[string]bool{}
	for _, row := range rows {
		if paths[row.Path] {
			t.Fatalf("duplicate path %q", row.Path)
		}
		paths[row.Path] = true
	}
}

func TestExtract_TranscriptionSelection(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "textparse", "testdata", "second.html"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err = os.Mkdir(filepath.Join(dir, "files"), 0700); err != nil {
		t.Fatal(err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	if err = os.WriteFile(filepath.Join(dir, "files", hash), data, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := selectFiles(dir, "1", []fetch.DocumentFile{{Name: "1_content.html", SHA256: hash, Size: int64(len(data))}}, absentOCR{})
	if err != nil || got.source != "html_transcription" || got.hash != hash {
		t.Fatalf("selection=%+v err=%v", got, err)
	}
	_, _, record := parseTranscription(got.data, got.name, got.hash)
	encoded, err := json.Marshal(record)
	if err != nil || !bytes.Contains(encoded, []byte(`"text_source":"html_transcription"`)) {
		t.Fatalf("text=%s err=%v", encoded, err)
	}
}

func TestExtract_Lineage(t *testing.T) {
	fetchHash := strings.Repeat("a", 64)
	for _, tc := range []struct {
		name, body, input string
		ok                bool
	}{
		{"legacy tree", "", fetchHash, true}, {"missing non-tree record", "", InputHash(fetchHash, "none"), false},
		{"html", `{"text_source":"html_transcription"}`, InputHash(fetchHash, "html_transcription"), true},
		{"none", `{"text_source":"none"}`, InputHash(fetchHash, "none"), true},
		{"mismatch", `{"text_source":"html_transcription"}`, fetchHash, false},
		{"invalid", `{"text_source":"unexpected"}`, fetchHash, false},
		{"malformed", `{`, fetchHash, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.body != "" {
				if err := os.WriteFile(filepath.Join(dir, "text.json"), []byte(tc.body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			err := fetch.CheckExtractLineage(dir, fetchHash, tc.input)
			if (err == nil) != tc.ok {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
