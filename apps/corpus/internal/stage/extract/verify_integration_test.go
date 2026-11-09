//go:build integration

package extract

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/traquydinh/apps/corpus/internal/artifact"
	"danny.vn/traquydinh/apps/corpus/internal/stage"
	"danny.vn/traquydinh/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestExtract_VerifiesFetchArtifact(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	url := testdb.New(t)
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := artifact.Store{Root: t.TempDir()}
	art, err := store.Publish(ctx, "fetch", strings.Repeat("a", 64), strings.Repeat("b", 64), func(dir string) error {
		for _, name := range []string{"tree.json", "detail.json", "diagram.json"} {
			if writeErr := os.WriteFile(filepath.Join(dir, name), []byte(`{"data":[]}`), 0600); writeErr != nil {
				return writeErr
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var runID, artifactID int64
	if queryErr := db.QueryRow(ctx, `INSERT INTO run (country,rel,state,snapshot) VALUES ('vn','20261109','running','{}') RETURNING id`).Scan(&runID); queryErr != nil {
		t.Fatal(queryErr)
	}
	if queryErr := db.QueryRow(ctx, `INSERT INTO artifact (stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('fetch','item',$1,$2,$3,$4) RETURNING id`, art.InputHash, strings.Repeat("b", 64), art.Path, art.SHA256).Scan(&artifactID); queryErr != nil {
		t.Fatal(queryErr)
	}
	if _, insertErr := db.Exec(ctx, `INSERT INTO run_item (run_id,stage,item_key,state,batch,artifact_id) VALUES ($1,'fetch','item','done',0,$2)`, runID, artifactID); insertErr != nil {
		t.Fatal(insertErr)
	}
	if writeErr := os.WriteFile(filepath.Join(store.Abs(art.Path), "tree.json"), []byte(`{"data":[],"changed":true}`), 0600); writeErr != nil {
		t.Fatal(writeErr)
	}
	_, err = New().Process(ctx, stage.Env{DB: db, Artifacts: store}, stage.Run{ID: runID}, stage.Item{Key: "item", InputHash: art.SHA256})
	var retryable *stage.RetryableError
	if !errors.As(err, &retryable) {
		t.Fatalf("expected retryable error: %v", err)
	}
}

func TestExtract_TreeTooLargeIsGap(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	url := testdb.New(t)
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	artifacts := artifact.Store{Root: t.TempDir()}
	tree := `{"success":true,"data":[` + strings.Repeat(`{"children":[`, 33) + strings.Repeat(`]}`, 33) + `]}`
	art, err := artifacts.Publish(ctx, "fetch", strings.Repeat("a", 64), strings.Repeat("b", 64), func(dir string) error {
		return os.WriteFile(filepath.Join(dir, "tree.json"), []byte(tree), 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	var runID, artifactID int64
	if queryErr := db.QueryRow(ctx, `INSERT INTO run (country,rel,state,snapshot) VALUES ('vn','20261109','running','{}') RETURNING id`).Scan(&runID); queryErr != nil {
		t.Fatal(queryErr)
	}
	if queryErr := db.QueryRow(ctx, `INSERT INTO artifact (stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('fetch','item',$1,$2,$3,$4) RETURNING id`, art.InputHash, strings.Repeat("b", 64), art.Path, art.SHA256).Scan(&artifactID); queryErr != nil {
		t.Fatal(queryErr)
	}
	if _, insertErr := db.Exec(ctx, `INSERT INTO run_item (run_id,stage,item_key,state,batch,artifact_id) VALUES ($1,'fetch','item','done',0,$2)`, runID, artifactID); insertErr != nil {
		t.Fatal(insertErr)
	}
	out, err := New().Process(ctx, stage.Env{DB: db, Artifacts: artifacts}, stage.Run{ID: runID}, stage.Item{Key: "item", InputHash: art.SHA256})
	if err != nil || out.State != "gap" || out.Artifact != nil || len(out.Gaps) != 1 || out.Gaps[0].Kind != "tree_too_large" {
		t.Fatalf("output = %+v, error = %v", out, err)
	}
}
