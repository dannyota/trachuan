//go:build integration

package chunk

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestChunk_SecondaryDocumentNoTreeNoChunksIntegration(t *testing.T) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	artifacts := artifact.Store{Root: t.TempDir()}
	meta := `{"version":1,"source_id":"1","source_url":"https://example.test/1","occurrences":[],"attachments":[]}`
	html := "fixture"
	hash := func(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }
	doc := fmt.Sprintf(`{"version":2,"document":{"key":"vn:chinhphu:1","country":"vn","source":"chinhphu","source_id":"1","source_url":"https://example.test/1","identity_evidence":[{"path":"detail.html","sha256":"%s"},{"path":"metadata.json","sha256":"%s"}]}}`, hash(html), hash(meta))
	fetchArt, err := artifacts.PublishContent(ctx, "fetch", strings.Repeat("a", 64), func(dir string) error {
		for name, data := range map[string]string{"document.json": doc, "metadata.json": meta, "detail.html": html, "files.json": `{"version":2,"files":[]}`} {
			if e := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	extractArt, err := artifacts.Publish(ctx, "extract", fetchArt.SHA256, strings.Repeat("b", 64), func(dir string) error { return os.WriteFile(filepath.Join(dir, "provisions.json"), []byte(`[]`), 0600) })
	if err != nil {
		t.Fatal(err)
	}
	var runID int64
	if err = db.QueryRow(ctx, `INSERT INTO run(country,rel,state,snapshot) VALUES ('vn','20990101','running','{}') RETURNING id`).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	insertChunkInput(t, ctx, db, runID, "vn:chinhphu:1", fetchArt, extractArt)
	env := stage.Env{DB: db, Artifacts: artifacts}
	out, err := New().Process(ctx, env, stage.Run{ID: runID}, stage.Item{Key: "vn:chinhphu:1", InputHash: inputHash("vn:chinhphu:1", fetchArt.SHA256, extractArt.SHA256)})
	if err != nil {
		t.Fatal(err)
	}
	if out.State != "done" || out.Artifact == nil {
		t.Fatalf("output=%+v", out)
	}
	file, coverage, err := ReadArtifact(artifacts.Abs(out.Artifact.Path))
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Chunks) != 0 || len(coverage.Gaps) != 1 || coverage.Gaps[0].Kind != "no_tree" {
		t.Fatalf("chunks=%+v coverage=%+v", file.Chunks, coverage)
	}
}

func insertChunkInput(t *testing.T, ctx context.Context, db *pgxpool.Pool, runID int64, key string, f, e artifact.Result) {
	t.Helper()
	for _, a := range []struct {
		name string
		art  artifact.Result
	}{{"fetch", f}, {"extract", e}} {
		var id int64
		if err := db.QueryRow(ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`, a.name, key, a.art.InputHash, strings.Repeat("c", 64), a.art.Path, a.art.SHA256).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) VALUES ($1,$2,$3,'done',0,$4)`, runID, a.name, key, id); err != nil {
			t.Fatal(err)
		}
	}
}

func TestChunkPlan_SkipsNonMembers(t *testing.T) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var runID int64
	if err = db.QueryRow(ctx, `INSERT INTO run(country,rel,state,snapshot) VALUES ('vn','20990102','running','{}') RETURNING id`).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"vn:vbpl:1", "vn:chinhphu:2"} {
		f := artifact.Result{InputHash: key + "-f", Path: key + "-f", SHA256: key + "-f"}
		e := artifact.Result{InputHash: f.SHA256, Path: key + "-e", SHA256: key + "-e"}
		insertChunkInput(t, ctx, db, runID, key, f, e)
	}
	replacement := "vn:vbpl:1"
	entries := []membership.Entry{{DocumentKey: replacement, Source: "vbpl", SourceID: "1", Class: "primary", Disposition: "active"}, {DocumentKey: "vn:chinhphu:2", Source: "chinhphu", SourceID: "2", Class: "secondary", Disposition: "source_replaced", ReplacementKey: &replacement}}
	if err = membership.PublishPhase(ctx, store.New(db), runID, membership.Final, entries); err != nil {
		t.Fatal(err)
	}
	run := stage.Run{ID: runID, Snapshot: stage.Snapshot{SourceList: []byte(`{"vbpl":{"selection_version":1}}`)}}
	items, err := New().Plan(ctx, stage.Env{DB: db}, run)
	if err != nil || len(items) != 1 || items[0].Key != replacement {
		t.Fatalf("items=%+v: %v", items, err)
	}
	run.Snapshot.SourceList = []byte(`{"vbpl":{"selection_version":0}}`)
	items, err = New().Plan(ctx, stage.Env{DB: db}, run)
	if err != nil || len(items) != 2 {
		t.Fatalf("legacy items=%+v: %v", items, err)
	}
	if _, err = db.Exec(ctx, `UPDATE run_scope SET final_hash='invalid' WHERE run_id=$1`, runID); err != nil {
		t.Fatal(err)
	}
	run.Snapshot.SourceList = []byte(`{"vbpl":{"selection_version":1}}`)
	if _, err = New().Plan(ctx, stage.Env{DB: db}, run); err == nil {
		t.Fatal("corrupt final membership accepted")
	}
}
