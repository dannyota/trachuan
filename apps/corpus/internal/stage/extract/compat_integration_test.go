//go:build integration

package extract

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestExtract_SourceTreeStageCompatibility(t *testing.T) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	artifacts := artifact.Store{Root: t.TempDir()}
	fetchArt, err := artifacts.PublishContent(ctx, "fetch", strings.Repeat("b", 64), func(dir string) error {
		for _, f := range []struct{ target, source string }{{"tree.json", "tree-67878.json"}, {"detail.json", "detail-67878.json"}} {
			data, e := os.ReadFile(filepath.Join("..", "..", "vbpl", "testdata", f.source))
			if e != nil {
				return e
			}
			if e = os.WriteFile(filepath.Join(dir, f.target), data, 0600); e != nil {
				return e
			}
		}
		return os.WriteFile(filepath.Join(dir, "files.json"), []byte(`[]`), 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	var runID, artifactID int64
	if err = db.QueryRow(ctx, `INSERT INTO run (country,rel,state,snapshot) VALUES ('vn','20990101','running','{}') RETURNING id`).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `INSERT INTO artifact (stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('fetch','vn:vbpl:67878',$1,$2,$3,$4) RETURNING id`, fetchArt.InputHash, strings.Repeat("b", 64), fetchArt.Path, fetchArt.SHA256).Scan(&artifactID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO run_item (run_id,stage,item_key,state,batch,artifact_id) VALUES ($1,'fetch','vn:vbpl:67878','done',0,$2)`, runID, artifactID); err != nil {
		t.Fatal(err)
	}
	env := stage.Env{DB: db, Artifacts: artifacts}
	run := stage.Run{ID: runID, Snapshot: stage.Snapshot{SourceList: []byte(`{"vbpl":{"selection_version":0}}`)}}
	extractor := New()
	items, err := extractor.Plan(ctx, env, run)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Key != "vn:vbpl:67878" || items[0].InputHash != fetchArt.SHA256 {
		t.Fatalf("items %+v", items)
	}
	out, err := extractor.Process(ctx, env, run, items[0])
	if err != nil {
		t.Fatal(err)
	}
	if out.Artifact == nil || out.State != "done" {
		t.Fatalf("output %+v", out)
	}
	if out.Artifact.SHA256 != "2e31d021f019a91eba44056aff135c7a7d0f5239dda95727cca7a494d5696b5f" {
		t.Fatalf("extract hash=%s", out.Artifact.SHA256)
	}
	if _, err = os.Stat(filepath.Join(artifacts.Abs(out.Artifact.Path), "text.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("text.json exists or cannot be checked: %v", err)
	}
	assertPinnedTreeChunk(t, artifacts, *out.Artifact)
}
