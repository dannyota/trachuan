package build

import (
	"os"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/stage/embed"
)

func TestVectors_ReconcilesOrdinals(t *testing.T) {
	file := chunk.File{Chunks: []chunk.Chunk{{Ordinal: 0, Content: "a"}, {Ordinal: 1, Content: "b"}, {Ordinal: 2, Content: "c"}}}
	idx := embed.Index{Rows: []embed.Row{{ChunkOrdinal: 0}, {ChunkOrdinal: 2}}, Rejected: []embed.Rejected{{ChunkOrdinal: 1}}}
	if err := reconcile(file, idx); err != nil {
		t.Fatal(err)
	}
	idx.Rejected = nil
	if err := reconcile(file, idx); err == nil {
		t.Fatal("missing ordinal accepted")
	}
}

func TestChunkLineage_RejectsWrongFetch(t *testing.T) {
	fetch, extract := "fetch", "extract"
	l := loader{items: map[string]map[string]InventoryItem{"fetch": {"key": {ArtifactSHA256: &fetch}}, "extract": {"key": {ArtifactSHA256: &extract}}}}
	if err := l.checkChunkLineage("key", chunk.File{FetchSHA256: "wrong", ExtractSHA256: extract}); err == nil {
		t.Fatal("expected lineage failure")
	}
}

func TestArtifactLineage_RejectsChangedInput(t *testing.T) {
	expected, actual := "expected", "actual"
	if err := checkInputHash(InventoryItem{InputHash: &actual}, expected); err == nil {
		t.Fatal("expected lineage error")
	}
}

func TestVectorSize_RejectsExtraBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vectors.f32")
	if err := os.WriteFile(path, []byte{1}, 0600); err != nil {
		t.Fatal(err)
	}
	if err := checkVectorSize(path, 0, 1024); err == nil {
		t.Fatal("expected vector size error")
	}
}

func TestChunkOwner_RejectsWrongAnchor(t *testing.T) {
	doc := loadedDocument{nodes: map[string]int64{"a": 1}, anchors: map[string]string{"a": "dieu-1"}}
	if _, err := doc.owner(chunk.Chunk{NodeID: "a", Anchor: "wrong"}); err == nil {
		t.Fatal("expected anchor error")
	}
}
