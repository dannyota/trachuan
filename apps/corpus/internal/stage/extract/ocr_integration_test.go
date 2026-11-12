//go:build integration

package extract

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestExtract_OCRStageIntegration(t *testing.T) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	pdf := fmt.Sprintf("%x", sha256.Sum256([]byte("pdf")))
	data, err := os.ReadFile(filepath.Join("..", "..", "textparse", "testdata", "ocr_decree_142_2026.txt"))
	if err != nil {
		t.Fatal(err)
	}
	artifacts := artifact.Store{Root: t.TempDir()}
	art, err := artifacts.PublishContent(ctx, "fetch", strings.Repeat("a", 64), func(dir string) error {
		for name, body := range map[string]string{"tree.json": `{"success":true,"data":[]}`, "detail.json": `{"data":{"id":"1"}}`, "files.json": fmt.Sprintf(`[{"name":"source.pdf","sha256":"%s","size":3}]`, pdf)} {
			if e := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var runID, artifactID int64
	if err = db.QueryRow(ctx, `INSERT INTO run(country,rel,state,snapshot) VALUES ('vn','20990105','running','{}') RETURNING id`).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('fetch','vn:vbpl:1',$1,$2,$3,$4) RETURNING id`, art.InputHash, strings.Repeat("a", 64), art.Path, art.SHA256).Scan(&artifactID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) VALUES ($1,'fetch','vn:vbpl:1','done',0,$2)`, runID, artifactID); err != nil {
		t.Fatal(err)
	}
	ocrDir := t.TempDir()
	ocr := ocrstore.Open(ocrDir, db)
	if _, err = ocr.Put(ctx, pdf, "vision-ocr/builtin-latest", "banhmi", pgtype.Int4{}, data); err != nil {
		t.Fatal(err)
	}
	extractor := New().WithOCR(ocr, nil, "checker")
	env := stage.Env{DB: db, Artifacts: artifacts}
	run := stage.Run{ID: runID, Snapshot: stage.Snapshot{SourceList: []byte(`{"vbpl":{"selection_version":0}}`)}}
	items, err := extractor.Plan(ctx, env, run)
	if err != nil || len(items) != 1 {
		t.Fatalf("plan: %v %v", items, err)
	}
	if items[0].InputHash != InputHash(art.SHA256, "none", "needs_ocr") {
		t.Fatalf("unchecked hash: %s", items[0].InputHash)
	}
	out, err := extractor.Process(ctx, env, run, items[0])
	if err != nil {
		t.Fatal(err)
	}
	resolution, err := extractor.Resolve(ctx, env, run, items[0], *out.Artifact)
	if err != nil || !hasGap(resolution.Gaps, "needs_ocr", "") {
		t.Fatalf("gaps=%v err=%v", resolution.Gaps, err)
	}
	entries, err := ocr.Lookup(ctx, []string{pdf}, []string{"vision-ocr/builtin-latest"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ocr.PutCheck(ctx, entries[pdf].Text, "checker", json.RawMessage(`{"same_document":true,"pages":10,"pdf_pages":10,"items":[{"kind":"amount","page":1,"ocr":"1","pdf":"2","note":"x"}],"verdict":"flagged"}`), 1, true); err != nil {
		t.Fatal(err)
	}
	next, err := extractor.Plan(ctx, env, run)
	if err != nil || len(next) != 1 || next[0].InputHash != items[0].InputHash {
		t.Fatalf("replayed plan: %v %v", next, err)
	}
	if _, err = extractor.Process(ctx, env, run, items[0]); err != nil {
		t.Fatalf("process frozen selection: %v", err)
	}
	if err = db.QueryRow(ctx, `INSERT INTO run(country,rel,state,snapshot) VALUES ('vn','20990106','running','{}') RETURNING id`).Scan(&run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) VALUES ($1,'fetch','vn:vbpl:1','done',0,$2)`, run.ID, artifactID); err != nil {
		t.Fatal(err)
	}
	next, err = extractor.Plan(ctx, env, run)
	if err != nil || len(next) != 1 || next[0].InputHash == items[0].InputHash {
		t.Fatalf("new run plan: %v %v", next, err)
	}
	out, err = extractor.Process(ctx, env, run, next[0])
	if err != nil {
		t.Fatal(err)
	}
	dir := artifacts.Abs(out.Artifact.Path)
	if err = fetch.CheckExtractLineage(dir, art.SHA256, next[0].InputHash); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "text.json"))
	if err != nil {
		t.Fatal(err)
	}
	var record textRecord
	if err = json.Unmarshal(b, &record); err != nil {
		t.Fatal(err)
	}
	if record.TextSource != "ocr" || record.FileName != "source.pdf" || record.SHA256 != pdf || record.Flagged == nil || *record.Flagged != 1 || len(record.Provisions) == 0 || record.Provisions[0].StartLine <= 0 {
		t.Fatal("OCR record is missing source metadata or line ranges")
	}
	resolution, err = extractor.Resolve(ctx, env, run, next[0], *out.Artifact)
	if err != nil || !hasGap(resolution.Gaps, "ocr_flagged", "") {
		t.Fatalf("gaps=%v err=%v", resolution.Gaps, err)
	}
	if err = os.WriteFile(filepath.Join(ocrDir, "text", record.TextSHA256+".txt"), []byte("altered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = extractor.Process(ctx, env, run, next[0]); err == nil {
		t.Fatal("accepted altered OCR text")
	}
}

func TestExtract_UncheckedEntryReplansArtifact(t *testing.T) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	pdf1 := fmt.Sprintf("%x", sha256.Sum256([]byte("one")))
	pdf2 := fmt.Sprintf("%x", sha256.Sum256([]byte("two")))
	artifacts := artifact.Store{Root: t.TempDir()}
	art, err := artifacts.PublishContent(ctx, "fetch", strings.Repeat("a", 64), func(dir string) error {
		for name, body := range map[string]string{"tree.json": `{"success":true,"data":[]}`, "detail.json": `{"data":{"id":"1"}}`, "files.json": fmt.Sprintf(`[{"name":"one.pdf","sha256":"%s","size":3},{"name":"two.pdf","sha256":"%s","size":3}]`, pdf1, pdf2)} {
			if writeErr := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); writeErr != nil {
				return writeErr
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var runID, artifactID int64
	if err = db.QueryRow(ctx, `INSERT INTO run(country,rel,state,snapshot) VALUES ('vn','20990105','running','{}') RETURNING id`).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('fetch','vn:vbpl:1',$1,$2,$3,$4) RETURNING id`, art.InputHash, strings.Repeat("a", 64), art.Path, art.SHA256).Scan(&artifactID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) VALUES ($1,'fetch','vn:vbpl:1','done',0,$2)`, runID, artifactID); err != nil {
		t.Fatal(err)
	}
	ocr := ocrstore.Open(t.TempDir(), db)
	extractor := New().WithOCR(ocr, nil, "checker")
	env := stage.Env{DB: db, Artifacts: artifacts}
	run := stage.Run{ID: runID, Snapshot: stage.Snapshot{SourceList: []byte(`{"vbpl":{"selection_version":0}}`)}}
	first, err := extractor.Plan(ctx, env, run)
	if err != nil || len(first) != 1 {
		t.Fatalf("first plan: %v %v", first, err)
	}
	before, err := extractor.Process(ctx, env, run, first[0])
	if err != nil {
		t.Fatal(err)
	}
	resolution, err := extractor.Resolve(ctx, env, run, first[0], *before.Artifact)
	if err != nil || !hasGap(resolution.Gaps, "text_source_ambiguous", "") {
		t.Fatalf("first gaps: %v %v", resolution.Gaps, err)
	}
	if _, err = ocr.Put(ctx, pdf1, "vision-ocr/builtin-latest", "banhmi", pgtype.Int4{}, []byte("text")); err != nil {
		t.Fatal(err)
	}
	next, err := extractor.Plan(ctx, env, run)
	if err != nil || len(next) != 1 || next[0].InputHash != first[0].InputHash {
		t.Fatalf("replayed plan: %v %v", next, err)
	}
	if _, err = extractor.Process(ctx, env, run, first[0]); err != nil {
		t.Fatalf("process frozen selection: %v", err)
	}
	if err = db.QueryRow(ctx, `INSERT INTO run(country,rel,state,snapshot) VALUES ('vn','20990106','running','{}') RETURNING id`).Scan(&run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) VALUES ($1,'fetch','vn:vbpl:1','done',0,$2)`, run.ID, artifactID); err != nil {
		t.Fatal(err)
	}
	next, err = extractor.Plan(ctx, env, run)
	if err != nil || len(next) != 1 || next[0].InputHash == first[0].InputHash {
		t.Fatalf("new run plan: %v %v", next, err)
	}
	after, err := extractor.Process(ctx, env, run, next[0])
	if err != nil {
		t.Fatal(err)
	}
	if after.Artifact.Path == before.Artifact.Path || after.Artifact.Adopted {
		t.Fatal("reused previous artifact")
	}
	resolution, err = extractor.Resolve(ctx, env, run, next[0], *after.Artifact)
	if err != nil || !hasGap(resolution.Gaps, "needs_ocr", "") {
		t.Fatalf("second gaps: %v %v", resolution.Gaps, err)
	}
	if err = fetch.CheckExtractLineage(artifacts.Abs(after.Artifact.Path), art.SHA256, next[0].InputHash); err != nil {
		t.Fatal(err)
	}
}

func TestExtract_FrozenHeadingAndRejected(t *testing.T) {
	for _, tc := range []struct {
		name, tree, result, initialGap, newGap, reason string
		flagged                                        int32
		preexisting                                    bool
	}{
		{"heading", bareTree, `{"same_document":true,"pages":1,"pdf_pages":1,"items":[],"verdict":"clean"}`, "tree_headings_only", "", "", 0, false},
		{"rejected", `{"success":true,"data":[]}`, `{"same_document":false,"pages":1,"pdf_pages":1,"items":[],"verdict":"rejected"}`, "needs_ocr", "ocr_rejected", "checker rejected OCR text", 0, true},
		{"article label", `{"success":true,"data":[]}`, `{"same_document":true,"pages":1,"pdf_pages":1,"items":[{"kind":"article_label","page":1,"ocr":"1","pdf":"2","note":"x"}],"verdict":"flagged"}`, "needs_ocr", "ocr_label_flagged", "checker flagged an article label or document number", 1, true},
		{"document number", `{"success":true,"data":[]}`, `{"same_document":true,"pages":1,"pdf_pages":1,"items":[{"kind":"document_number","page":1,"ocr":"1","pdf":"2","note":"x"}],"verdict":"flagged"}`, "needs_ocr", "ocr_label_flagged", "checker flagged an article label or document number", 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := pgxpool.New(ctx, testdb.New(t))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			pdf := fmt.Sprintf("%x", sha256.Sum256([]byte("pdf")))
			text, err := os.ReadFile(filepath.Join("..", "..", "textparse", "testdata", "ocr_decree_142_2026.txt"))
			if err != nil {
				t.Fatal(err)
			}
			artifacts := artifact.Store{Root: t.TempDir()}
			art, err := artifacts.PublishContent(ctx, "fetch", strings.Repeat("a", 64), func(dir string) error {
				for name, body := range map[string]string{"tree.json": tc.tree, "detail.json": `{"data":{"id":"1"}}`, "files.json": fmt.Sprintf(`[{"name":"x.pdf","sha256":"%s","size":3}]`, pdf)} {
					if writeErr := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); writeErr != nil {
						return writeErr
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			var artifactID int64
			if err = db.QueryRow(ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('fetch','vn:vbpl:1',$1,$2,$3,$4) RETURNING id`, art.InputHash, strings.Repeat("a", 64), art.Path, art.SHA256).Scan(&artifactID); err != nil {
				t.Fatal(err)
			}
			makeRun := func(rel string) stage.Run {
				t.Helper()
				var id int64
				if e := db.QueryRow(ctx, `INSERT INTO run(country,rel,state,snapshot) VALUES ('vn',$1,'running','{}') RETURNING id`, rel).Scan(&id); e != nil {
					t.Fatal(e)
				}
				if _, e := db.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) VALUES ($1,'fetch','vn:vbpl:1','done',0,$2)`, id, artifactID); e != nil {
					t.Fatal(e)
				}
				return stage.Run{ID: id, Snapshot: stage.Snapshot{SourceList: []byte(`{"vbpl":{"selection_version":0}}`)}}
			}
			ocr := ocrstore.Open(t.TempDir(), db)
			put := func() {
				t.Helper()
				if _, e := ocr.Put(ctx, pdf, "vision-ocr/builtin-latest", "banhmi", pgtype.Int4{}, text); e != nil {
					t.Fatal(e)
				}
			}
			if tc.preexisting {
				put()
			}
			extractor := New().WithOCR(ocr, nil, "checker")
			env := stage.Env{DB: db, Artifacts: artifacts}
			run := makeRun("20990101")
			first, err := extractor.Plan(ctx, env, run)
			if err != nil || len(first) != 1 {
				t.Fatalf("plan: %v %v", first, err)
			}
			if !tc.preexisting {
				put()
			}
			entries, err := ocr.Lookup(ctx, []string{pdf}, []string{"vision-ocr/builtin-latest"}, "")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = ocr.PutCheck(ctx, entries[pdf].Text, "checker", json.RawMessage(tc.result), tc.flagged, true); err != nil {
				t.Fatal(err)
			}
			replay, err := extractor.Plan(ctx, env, run)
			if err != nil || len(replay) != 1 || replay[0].InputHash != first[0].InputHash {
				t.Fatalf("replay: %v %v", replay, err)
			}
			out, err := extractor.Process(ctx, env, run, first[0])
			if err != nil {
				t.Fatal(err)
			}
			resolved, err := extractor.Resolve(ctx, env, run, first[0], *out.Artifact)
			if err != nil || !hasGap(resolved.Gaps, tc.initialGap, "") {
				t.Fatalf("initial gap: %v %v", resolved.Gaps, err)
			}
			nextRun := makeRun("20990102")
			next, err := extractor.Plan(ctx, env, nextRun)
			if err != nil || len(next) != 1 || next[0].InputHash == first[0].InputHash {
				t.Fatalf("new run: %v %v", next, err)
			}
			out, err = extractor.Process(ctx, env, nextRun, next[0])
			if err != nil {
				t.Fatal(err)
			}
			resolved, err = extractor.Resolve(ctx, env, nextRun, next[0], *out.Artifact)
			if err != nil {
				t.Fatal(err)
			}
			if tc.newGap == "ocr_label_flagged" {
				data, readErr := os.ReadFile(filepath.Join(artifacts.Abs(out.Artifact.Path), "provisions.json"))
				if readErr != nil || string(data) != "[]" {
					t.Fatalf("provisions=%s err=%v", data, readErr)
				}
				if err = fetch.CheckExtractLineage(artifacts.Abs(out.Artifact.Path), art.SHA256, next[0].InputHash); err != nil {
					t.Fatal(err)
				}
			}
			if tc.newGap != "" && (!hasGap(resolved.Gaps, tc.newGap, tc.reason) || !hasGap(resolved.Gaps, "no_tree", "")) {
				t.Fatalf("new gap: %v", resolved.Gaps)
			}
		})
	}
}
