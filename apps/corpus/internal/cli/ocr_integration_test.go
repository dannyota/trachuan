//go:build integration

package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestImport_FetchBundles(t *testing.T) {
	ctx := context.Background()
	dbURL := testdb.New(t)
	db, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	artifacts := artifact.Store{Root: t.TempDir()}
	hash := func(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }
	pdf, nonPDF, pending := hash("pdf"), hash("doc"), hash("pending")
	var runID int64
	if err = db.QueryRow(ctx, `INSERT INTO run (country,rel,state,snapshot) VALUES ('vn','20990101','running','{}') RETURNING id`).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ id, state, files string }{
		{"1", "done", fmt.Sprintf(`[{"name":"source.PDF","sha256":"%s","size":3},{"name":"other.doc","sha256":"%s","size":3}]`, pdf, nonPDF)},
		{"2", "pending", fmt.Sprintf(`[{"name":"source.pdf","sha256":"%s","size":3}]`, pending)},
	} {
		art, err := artifacts.PublishContent(ctx, "fetch", strings.Repeat("b", 64), func(dir string) error {
			if err := os.WriteFile(filepath.Join(dir, "detail.json"), []byte(fmt.Sprintf(`{"data":{"id":"%s"}}`, item.id)), 0600); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(dir, "files.json"), []byte(item.files), 0600)
		})
		if err != nil {
			t.Fatal(err)
		}
		key := "vn:vbpl:" + item.id
		var artifactID int64
		if err = db.QueryRow(ctx, `INSERT INTO artifact (stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('fetch',$1,$2,$3,$4,$5) RETURNING id`, key, art.InputHash, strings.Repeat("b", 64), art.Path, art.SHA256).Scan(&artifactID); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, `INSERT INTO run_item (run_id,stage,item_key,state,batch,artifact_id) VALUES ($1,'fetch',$2,$3,0,$4)`, runID, key, item.state, artifactID); err != nil {
			t.Fatal(err)
		}
	}
	importDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(importDir, "ocr"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{pdf, nonPDF, pending} {
		if err := os.WriteFile(filepath.Join(importDir, "ocr", name+".txt"), []byte("text"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("CORPUS_DATABASE_URL", dbURL)
	t.Setenv("CORPUS_ARTIFACTS", artifacts.Root)
	t.Setenv("CORPUS_OCR_STORE", t.TempDir())
	var out, stderr bytes.Buffer
	if code := Run(ctx, []string{"ocr", "import-banhmi", "vn", "20990101", importDir}, &out, &stderr); code != 0 {
		t.Fatalf("status %d: %s", code, stderr.String())
	}
	if out.String() != "imported: 1\nalready present: 0\nnot in the run: 2\nrejected: 0\n" {
		t.Fatalf("counts: %s", out.String())
	}
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM ocr_text WHERE pdf_sha256=$1 AND engine_id='vision-ocr/builtin-latest' AND origin='banhmi' AND pages IS NULL`, pdf).Scan(&count); err != nil || count != 1 {
		t.Fatalf("row count %d: %v", count, err)
	}
}
