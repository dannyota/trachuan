//go:build integration

package build

import (
	"context"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
)

func TestLoad_ExtractSelectionLineage(t *testing.T) {
	f := fixture(t)
	key := "vn:vbpl:1"
	art := f.fetch(t, key)
	input := fetch.ExtractInputHash(art.SHA256, "html_transcription")
	nodes := []chunk.Node{{NodeID: "tx:1234567890abcdef", Kind: "dieu", Label: "Điều 1", Ordinal: 1, Path: "dieu-1", Text: "Điều 1. Nội dung văn bản."}}
	f.addArtifact(t, "extract", key, map[string]any{"text.json": map[string]string{"text_source": "html_transcription"}, "provisions.json": nodes}, input)
	counts, err := f.load(t)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Provisions != 1 {
		t.Fatalf("provisions: %d", counts.Provisions)
	}
	var anchor string
	if err = f.corpus.QueryRow(context.Background(), `SELECT anchor FROM provision`).Scan(&anchor); err != nil || anchor != "dieu-1" {
		t.Fatalf("anchor %q: %v", anchor, err)
	}
}
