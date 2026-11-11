package extract

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
)

func TestExtract_SourceTreeArtifactBytesUnchanged(t *testing.T) {
	ctx := context.Background()
	rows, gaps, err := extractTree(ctx, fixture(t, "tree-67878.json"))
	if err != nil {
		t.Fatal(err)
	}
	store := artifact.Store{Root: t.TempDir()}
	art, err := store.Publish(ctx, "extract", strings.Repeat("a", 64), strings.Repeat("b", 64), func(dir string) error {
		for _, f := range []struct {
			name  string
			value any
		}{{"provisions.json", rows}, {"report.json", gaps}} {
			b, e := json.Marshal(f.value)
			if e != nil {
				return e
			}
			if e = os.WriteFile(filepath.Join(dir, f.name), b, 0600); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	chunkInput := stage.RecipeHash("chunk-input", "1", "vn:vbpl:67878", strings.Repeat("a", 64), art.SHA256)
	if art.SHA256 != "2e31d021f019a91eba44056aff135c7a7d0f5239dda95727cca7a494d5696b5f" || chunkInput != "79bb4f26dbfed247351b30aef04eea9bb9572477ddb4649776131762ae844c5c" {
		t.Fatalf("extract=%s chunk_input=%s", art.SHA256, chunkInput)
	}
	bundle := t.TempDir()
	detail, err := os.ReadFile(filepath.Join("..", "..", "vbpl", "testdata", "detail-67878.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(bundle, "detail.json"), detail, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(bundle, "files.json"), []byte(`[]`), 0600); err != nil {
		t.Fatal(err)
	}
	doc, err := fetch.ReadDocument(bundle, "vn:vbpl:67878")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	var nodes []chunk.Node
	if err = json.Unmarshal(data, &nodes); err != nil {
		t.Fatal(err)
	}
	file, coverage, err := chunk.Split(nodes, chunk.Meta{Key: doc.Key, Number: doc.Number, Title: doc.Title, EffFrom: doc.EffFrom, IssueDate: doc.IssueDate}, chunk.Recipe1())
	if err != nil {
		t.Fatal(err)
	}
	file.FetchSHA256, file.ExtractSHA256 = strings.Repeat("a", 64), art.SHA256
	chunkArt, err := store.Publish(ctx, "chunk", chunkInput, chunk.Recipe1().Hash(), func(dir string) error {
		for _, v := range []struct {
			name  string
			value any
		}{{"chunks.json", file}, {"coverage.json", coverage}} {
			b, e := json.Marshal(v.value)
			if e != nil {
				return e
			}
			if e = os.WriteFile(filepath.Join(dir, v.name), b, 0600); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if chunkArt.SHA256 != "19301b43d39e135a09337a37849543e99f464da0dce64945b0f109c6827e022a" {
		t.Fatalf("chunk manifest=%s", chunkArt.SHA256)
	}

}
