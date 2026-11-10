//go:build integration

package chunk

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestChunkStage_RowsAndResolveGaps(t *testing.T) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	artifacts := artifact.Store{Root: t.TempDir()}
	var extractID int64
	extractHash := strings.Repeat("a", 64)
	err = db.QueryRow(ctx, `INSERT INTO artifact (stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('extract','vn:vbpl:1',$1,$2,'extract',$3) RETURNING id`, strings.Repeat("b", 64), strings.Repeat("c", 64), extractHash).Scan(&extractID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO provision (artifact_id,node_id,kind,label,heading,ordinal,path,text) VALUES ($1,'a','dieu','','',0,'dieu-1','A substantive provision with sufficient text.')`, extractID)
	if err != nil {
		t.Fatal(err)
	}
	file, coverage, err := Split([]Node{{NodeID: "a", Kind: "dieu", Path: "dieu-1", Text: "A substantive provision with sufficient text."}}, Meta{Key: "vn:vbpl:1"}, Recipe1())
	if err != nil {
		t.Fatal(err)
	}
	file.ExtractSHA256 = extractHash
	coverage.Gaps = append(coverage.Gaps, Gap{Kind: "chunk_omission"}, Gap{Kind: "no_chunks"})
	art, err := artifacts.Publish(ctx, "chunk", strings.Repeat("d", 64), strings.Repeat("e", 64), func(dir string) error {
		for name, value := range map[string]any{"chunks.json": file, "coverage.json": coverage} {
			data, marshalErr := json.Marshal(value)
			if marshalErr != nil {
				return marshalErr
			}
			if writeErr := os.WriteFile(filepath.Join(dir, name), data, 0600); writeErr != nil {
				return writeErr
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var chunkID int64
	err = db.QueryRow(ctx, `INSERT INTO artifact (stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('chunk','vn:vbpl:1',$1,$2,$3,$4) RETURNING id`, art.InputHash, strings.Repeat("e", 64), art.Path, art.SHA256).Scan(&chunkID)
	if err != nil {
		t.Fatal(err)
	}
	env := stage.Env{DB: db, Artifacts: artifacts}
	rows, err := New().Rows(ctx, env, art)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = rows(ctx, tx, chunkID); err != nil {
		t.Fatal(err)
	}
	var gotExtractID int64
	var nodeID string
	if err = tx.QueryRow(ctx, `SELECT extract_artifact_id,node_id FROM chunk WHERE artifact_id=$1 AND ordinal=0`, chunkID).Scan(&gotExtractID, &nodeID); err != nil {
		t.Fatal(err)
	}
	if gotExtractID != extractID || nodeID != "a" {
		t.Fatalf("unexpected projection: %d %q", gotExtractID, nodeID)
	}
	if _, err = tx.Exec(ctx, `UPDATE chunk SET node_id='missing' WHERE artifact_id=$1`, chunkID); err == nil {
		t.Fatal("composite provision foreign key accepted missing node")
	}
	resolution, err := New().Resolve(ctx, env, stage.Run{}, stage.Item{}, art)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, gap := range resolution.Gaps {
		got[gap.Kind] = true
	}
	if !got["chunk_omission"] || !got["no_chunks"] {
		t.Fatalf("missing gaps: %#v", resolution.Gaps)
	}
}

func TestGetExtractArtifactByHash_ReturnsLowestID(t *testing.T) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var first, second int64
	hash := strings.Repeat("a", 64)
	for i, id := range []*int64{&second, &first} {
		err = db.QueryRow(ctx, `INSERT INTO artifact (id,stage,item_key,input_hash,recipe_hash,path,sha256) OVERRIDING SYSTEM VALUE VALUES ($5,'extract','vn:vbpl:1',$1,$2,$3,$4) RETURNING id`, strings.Repeat(string(rune('b'+i)), 64), strings.Repeat("d", 64), string(rune('a'+i)), hash, 2-i).Scan(id)
		if err != nil {
			t.Fatal(err)
		}
	}
	q := store.New(db)
	for i := 0; i < 10; i++ {
		got, e := q.GetExtractArtifactByHash(ctx, store.GetExtractArtifactByHashParams{Sha256: hash, ItemKey: "vn:vbpl:1"})
		if e != nil || got != first {
			t.Fatalf("got %d, want %d: %v", got, first, e)
		}
	}
}
