package chunk

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChunk_VBPLArtifactBytesUnchanged(t *testing.T) {
	dir := t.TempDir()
	detail := `{"data":{"id":"1","docNum":"1/2024/TT","title":"Fixture title","effFrom":"2024-07-01","issueDate":"2024-01-01"}}`
	if err := os.WriteFile(filepath.Join(dir, "detail.json"), []byte(detail), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "files.json"), []byte(`[]`), 0600); err != nil {
		t.Fatal(err)
	}
	extract := t.TempDir()
	nodes := []Node{{NodeID: "article", Kind: "dieu", Label: "Điều 1", Heading: "Purpose", Ordinal: 1, Path: "dieu-1", Text: "A substantive provision with sufficient text."}}
	data, err := json.Marshal(nodes)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(extract, "provisions.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	file, coverage, err := chunkInputs(dir, extract, "vn:vbpl:1")
	if err != nil {
		t.Fatal(err)
	}
	file.FetchSHA256, file.ExtractSHA256 = "fetch", "extract"
	b, err := json.Marshal([]any{file, coverage})
	if err != nil {
		t.Fatal(err)
	}
	const want = `[{"version":1,"document_key":"vn:vbpl:1","fetch_sha256":"fetch","extract_sha256":"extract","chunks":[{"ordinal":0,"node_id":"article","anchor":"dieu-1","context_prefix":"1/2024/TT\nFixture title\nHiệu lực: 2024-07-01","content":"A substantive provision with sufficient text.","rough_tokens":22,"spans":[{"node_id":"article","start":0,"end":45,"content_start":0,"content_end":45,"role":"body"}]}]},{"version":1,"document_key":"vn:vbpl:1","gaps":[],"omissions":[],"covered_ranges":[{"node_id":"article","start":0,"end":45}],"whitespace_ranges":[{"node_id":"article","start":1,"end":2},{"node_id":"article","start":13,"end":14},{"node_id":"article","start":23,"end":24},{"node_id":"article","start":28,"end":29},{"node_id":"article","start":39,"end":40}]}]`
	if Recipe2().Hash() != "40c50d47068734972075a0a0a6cd78b1e5dc55fc43bfb4dfce34944049336d82" {
		t.Fatalf("recipe hash changed: %s", Recipe2().Hash())
	}
	if string(b) != want {
		t.Fatalf("artifact=%s\nrecipe=%s", b, Recipe2().Hash())
	}
}

func TestChunk_SecondaryDocumentNoTreeNoChunks(t *testing.T) {
	dir, extract := t.TempDir(), t.TempDir()
	meta := `{"version":1,"source_id":"1","source_url":"https://example.test/1","occurrences":[],"attachments":[]}`
	html := "fixture"
	hash := func(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }
	doc := fmt.Sprintf(`{"version":2,"document":{"key":"vn:chinhphu:1","country":"vn","source":"chinhphu","source_id":"1","source_url":"https://example.test/1","identity_evidence":[{"path":"detail.html","sha256":"%s"},{"path":"metadata.json","sha256":"%s"}]}}`, hash(html), hash(meta))
	for name, body := range map[string]string{"document.json": doc, "metadata.json": meta, "detail.html": html, "files.json": `{"version":2,"files":[]}`} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(extract, "provisions.json"), []byte(`[]`), 0600); err != nil {
		t.Fatal(err)
	}
	file, coverage, err := chunkInputs(dir, extract, "vn:chinhphu:1")
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Chunks) != 0 || len(coverage.Gaps) != 1 || coverage.Gaps[0].Kind != "no_tree" {
		t.Fatalf("chunks=%+v coverage=%+v", file.Chunks, coverage)
	}
}

func TestChunk_GeneratedProvisionsChunked(t *testing.T) {
	chapter := "tx:chapter"
	nodes := []Node{
		{NodeID: chapter, Kind: "chuong", Label: "Chương I", Heading: "Quy định chung", Ordinal: 1, Path: "kem-theo-1/chuong-i", Text: "Chương I\nQuy định chung"},
		{NodeID: "tx:article", ParentNodeID: &chapter, Kind: "dieu", Label: "Điều 1", Ordinal: 2, Path: "kem-theo-1/dieu-1", Text: "Điều 1. Nội dung của quy định có đủ chữ để lập một đoạn văn bản."},
		{NodeID: "tx:neutral", Kind: "other", Ordinal: 3, Path: "doan-1", Text: "Một đoạn văn bản trung lập có đủ nội dung để tạo thành đoạn trích."},
	}
	file, _, err := Split(nodes, Meta{Key: "vn:vbpl:1", Number: "123", Title: "Văn bản"}, Recipe2())
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, c := range file.Chunks {
		seen[c.Anchor] = true
		if c.Anchor == "kem-theo-1/dieu-1" && !strings.Contains(c.ContextPrefix, "Chương I Quy định chung") {
			t.Fatalf("prefix %q", c.ContextPrefix)
		}
	}
	if !seen["kem-theo-1/dieu-1"] || !seen["doan-1"] {
		t.Fatalf("chunks=%+v", file.Chunks)
	}
}

func TestChunk_MissingTextRecordIsAccepted(t *testing.T) {
	fetchHash := strings.Repeat("a", 64)
	if err := checkExtractLineage(t.TempDir(), fetchHash, fetchHash); err != nil {
		t.Fatalf("missing record: %v", err)
	}
}
