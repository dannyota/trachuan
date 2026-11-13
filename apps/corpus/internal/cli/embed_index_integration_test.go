//go:build integration

package cli

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/embed"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"danny.vn/trachuan/internal/embedding"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestEmbedIndex_IdempotentAndSkipsInvalidArtifacts(t *testing.T) {
	ctx := context.Background()
	url := testdb.New(t)
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	t.Setenv("CORPUS_DATABASE_URL", url)
	root := t.TempDir()
	t.Setenv("CORPUS_ARTIFACTS", root)
	artifacts := artifact.Store{Root: root}
	r := embedding.DocumentRecipe1()
	s := embed.New(nil, r)
	recipe := s.Recipe(stage.Snapshot{})
	input := strings.Repeat("a", 64)
	hash := strings.Repeat("b", 64)
	idx := embed.Index{Version: 1, DocumentKey: "doc", ChunkSHA256: input, RecipeHash: r.Hash(), Recipe: r, Dimensions: r.Dimensions, Rows: []embed.Row{{Row: 0, ChunkOrdinal: 0, TokenCount: 2, InputSHA256: hash}}, Rejected: []embed.Rejected{}}
	art, err := artifacts.Publish(ctx, "embed", input, recipe, func(dir string) error {
		data, e := json.Marshal(idx)
		if e != nil {
			return e
		}
		if e = os.WriteFile(filepath.Join(dir, "index.json"), data, 0600); e != nil {
			return e
		}
		vectors := make([]byte, r.Dimensions*4)
		binary.LittleEndian.PutUint32(vectors, 0x3f800000)
		return os.WriteFile(filepath.Join(dir, "vectors.f32"), vectors, 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, "INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('embed','doc',$1,$2,$3,$4),('embed','bad',$5,$2,'missing',$6)", input, recipe, art.Path, art.SHA256, strings.Repeat("c", 64), strings.Repeat("d", 64)); err != nil {
		t.Fatal(err)
	}
	for i, expected := range []string{"rows inserted=1", "rows inserted=0"} {
		var out, errout bytes.Buffer
		if code := Run(ctx, []string{"embed-index"}, &out, &errout); code != 0 {
			t.Fatalf("run %d code=%d stderr=%s", i, code, errout.String())
		}
		if !strings.Contains(out.String(), expected) || !strings.Contains(out.String(), "artifacts verified=1") || !strings.Contains(out.String(), "artifacts skipped=1") || !strings.Contains(out.String(), "skipped:") {
			t.Fatalf("output=%s", out.String())
		}
	}
}
