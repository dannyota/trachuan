//go:build integration

package worker

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/extract"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"danny.vn/trachuan/apps/corpus/internal/workflow"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestWorker_ConfiguresOCRStore(t *testing.T) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	hash := func(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }
	one, two := hash("one"), hash("two")
	root := t.TempDir()
	if _, putErr := ocrstore.Open(root, db).Put(ctx, one, "vision-ocr/builtin-latest", "banhmi", pgtype.Int4{}, []byte("OCR")); putErr != nil {
		t.Fatal(putErr)
	}
	artifacts := artifact.Store{Root: t.TempDir()}
	art, err := artifacts.PublishContent(ctx, "fetch", strings.Repeat("a", 64), func(dir string) error {
		for name, data := range map[string]string{"tree.json": `{"success":true,"data":[]}`, "detail.json": `{"data":{"id":"1"}}`, "files.json": fmt.Sprintf(`[{"name":"one.pdf","sha256":"%s","size":3},{"name":"two.pdf","sha256":"%s","size":3}]`, one, two)} {
			if writeErr := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); writeErr != nil {
				return writeErr
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var runID, artID int64
	if err = db.QueryRow(ctx, `INSERT INTO run(country,rel,state,snapshot) VALUES ('vn','20990105','running','{}') RETURNING id`).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('fetch','vn:vbpl:1',$1,$2,$3,$4) RETURNING id`, art.InputHash, strings.Repeat("a", 64), artifacts.Abs(art.Path), art.SHA256).Scan(&artID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) VALUES ($1,'fetch','vn:vbpl:1','done',0,$2)`, runID, artID); err != nil {
		t.Fatal(err)
	}
	run := stage.Run{ID: runID, Snapshot: stage.Snapshot{SourceList: []byte(`{"vbpl":{"selection_version":0}}`)}}
	for _, tc := range []struct{ name, root, gap string }{{"unset", "", "text_source_ambiguous"}, {"set", root, "needs_ocr"}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CORPUS_OCR_STORE", tc.root)
			env := stage.Env{DB: db, Artifacts: artifact.Store{Root: t.TempDir()}}
			extractor := extract.New()
			registry := workflow.NewRegistry(extractor)
			configureOCR(registry, db)
			items, err := extractor.Plan(ctx, env, run)
			if err != nil || len(items) != 1 {
				t.Fatalf("plan: %v %v", items, err)
			}
			out, err := extractor.Process(ctx, env, run, items[0])
			if err != nil {
				t.Fatal(err)
			}
			resolved, err := extractor.Resolve(ctx, env, run, items[0], *out.Artifact)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, g := range resolved.Gaps {
				if g.Kind == tc.gap {
					found = true
				}
			}
			if !found {
				t.Fatalf("want gap %s, got %v", tc.gap, resolved.Gaps)
			}
		})
	}
}
