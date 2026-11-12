//go:build integration

package relate

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

func TestRelate_VerifiesFetchArtifact(t *testing.T) {
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
	if _, insertErr := db.Exec(ctx, `INSERT INTO run_item (run_id,stage,item_key,state,batch) VALUES ($1,'extract','item','gap',0)`, runID); insertErr != nil {
		t.Fatal(insertErr)
	}
	run := stage.Run{ID: runID, Snapshot: stage.Snapshot{SourceList: []byte(`{}`)}}
	for _, tc := range []struct{ state, extract string }{{"gap", "-"}, {"done", art.SHA256}} {
		var id any
		if tc.state == "done" {
			id = artifactID
		}
		if _, err = db.Exec(ctx, `UPDATE run_item SET state=$2, artifact_id=$3 WHERE run_id=$1 AND stage='extract' AND item_key='item'`, runID, tc.state, id); err != nil {
			t.Fatal(err)
		}
		items, planErr := New().Plan(ctx, stage.Env{DB: db}, run)
		if planErr != nil || len(items) != 1 || items[0].InputHash != PairInputHash(art.SHA256, tc.extract) {
			t.Fatalf("%s: items=%+v error=%v", tc.state, items, planErr)
		}
	}
	if _, err = db.Exec(ctx, `UPDATE run_item SET state='pending', artifact_id=NULL WHERE run_id=$1 AND stage='extract' AND item_key='item'`, runID); err != nil {
		t.Fatal(err)
	}
	items, err := New().Plan(ctx, stage.Env{DB: db}, run)
	if err != nil || len(items) != 0 {
		t.Fatalf("pending: items=%+v error=%v", items, err)
	}
	if _, err = db.Exec(ctx, `UPDATE run_item SET state='gap', artifact_id=NULL WHERE run_id=$1 AND stage='extract' AND item_key='item'`, runID); err != nil {
		t.Fatal(err)
	}
	if writeErr := os.WriteFile(filepath.Join(store.Abs(art.Path), "tree.json"), []byte(`{"data":[],"changed":true}`), 0600); writeErr != nil {
		t.Fatal(writeErr)
	}
	_, err = New().Process(ctx, stage.Env{DB: db, Artifacts: store}, stage.Run{ID: runID}, stage.Item{Key: "item", InputHash: PairInputHash(art.SHA256, "-")})
	var retryable *stage.RetryableError
	if !errors.As(err, &retryable) {
		t.Fatalf("expected retryable error: %v", err)
	}
}
