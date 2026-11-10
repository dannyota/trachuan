package chunk

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	if Recipe1().Hash() != "620e0ae95a786f399500349dec089c779fba9900f84d52e6000a34152a176cb7" {
		t.Fatalf("recipe hash changed: %s", Recipe1().Hash())
	}
	if string(b) != want {
		t.Fatalf("artifact=%s\nrecipe=%s", b, Recipe1().Hash())
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
