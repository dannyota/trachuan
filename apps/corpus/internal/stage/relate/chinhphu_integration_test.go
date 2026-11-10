//go:build integration

package relate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRelate_ChinhphuEmptySet(t *testing.T) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	artifacts := artifact.Store{Root: t.TempDir()}
	art, err := artifacts.PublishContent(ctx, "fetch", strings.Repeat("b", 64), func(dir string) error {
		return os.WriteFile(filepath.Join(dir, "document.json"), []byte(`{"version":2}`), 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	var runID, artifactID int64
	err = db.QueryRow(ctx, `INSERT INTO run (country,rel,state,snapshot) VALUES ('vn','20990101','running','{}') RETURNING id`).Scan(&runID)
	if err != nil {
		t.Fatal(err)
	}
	err = db.QueryRow(ctx, `INSERT INTO artifact (stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('fetch','vn:chinhphu:1',$1,$2,$3,$4) RETURNING id`, art.InputHash, strings.Repeat("b", 64), art.Path, art.SHA256).Scan(&artifactID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO run_item (run_id,stage,item_key,state,batch,artifact_id) VALUES ($1,'fetch','vn:chinhphu:1','done',0,$2)`, runID, artifactID)
	if err != nil {
		t.Fatal(err)
	}
	out, err := New().Process(ctx, stage.Env{DB: db, Artifacts: artifacts}, stage.Run{ID: runID}, stage.Item{Key: "vn:chinhphu:1", InputHash: art.SHA256})
	if err != nil {
		t.Fatal(err)
	}
	if out.Artifact == nil || out.State != "done" {
		t.Fatalf("output %+v", out)
	}
	rows, err := os.ReadFile(filepath.Join(artifacts.Abs(out.Artifact.Path), "relations.json"))
	if err != nil || string(rows) != "[]" {
		t.Fatalf("relations %s: %v", rows, err)
	}
	evidence, err := os.ReadFile(filepath.Join(artifacts.Abs(out.Artifact.Path), "evidence.json"))
	if err != nil || !strings.Contains(string(evidence), "not_parsed") {
		t.Fatalf("evidence %s: %v", evidence, err)
	}
}
