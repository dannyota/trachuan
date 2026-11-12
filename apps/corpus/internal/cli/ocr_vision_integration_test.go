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
	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/ocrvision"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func visionRunFixture(t *testing.T) (*checkRun, []string) {
	t.Helper()
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	var runID int64
	if err = db.QueryRow(ctx, `INSERT INTO run (country,rel,state,snapshot) VALUES ('vn','20990101','running','{}') RETURNING id`).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	env := &checkRun{db: db, root: t.TempDir(), artifacts: artifact.Store{Root: t.TempDir()}, run: stage.Run{ID: runID, Country: "vn", Rel: "20990101", Snapshot: stage.Snapshot{SourceList: []byte(`{"vbpl":{"selection_version":0}}`)}}}
	var hashes []string
	for i, gap := range []string{"needs_ocr", "needs_ocr", "needs_ocr", "no_text_source", "needs_ocr"} {
		key := fmt.Sprintf("vn:vbpl:%d", i+1)
		data := visionTestPDF(i)
		hash := fmt.Sprintf("%x", sha256.Sum256(data))
		hashes = append(hashes, hash)
		art, err := env.artifacts.PublishContent(ctx, "fetch", strings.Repeat("b", 64), func(dir string) error {
			for name, body := range map[string][]byte{"detail.json": []byte(fmt.Sprintf(`{"data":{"id":"%d"}}`, i+1)), "tree.json": []byte(`{"success":true,"data":[]}`), "files.json": []byte(fmt.Sprintf(`[{"name":"source.pdf","sha256":"%s","size":%d}]`, hash, len(data)))} {
				if err := os.WriteFile(filepath.Join(dir, name), body, 0600); err != nil {
					return err
				}
			}
			if err := os.Mkdir(filepath.Join(dir, "files"), 0700); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(dir, "files", hash), data, 0600)
		})
		if err != nil {
			t.Fatal(err)
		}
		var artifactID int64
		if err = db.QueryRow(ctx, `INSERT INTO artifact (stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('fetch',$1,$2,$3,$4,$5) RETURNING id`, key, art.InputHash, strings.Repeat("b", 64), art.Path, art.SHA256).Scan(&artifactID); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, `INSERT INTO run_item (run_id,stage,item_key,state,batch,artifact_id) VALUES ($1,'fetch',$2,'done',0,$3)`, runID, key, artifactID); err != nil {
			t.Fatal(err)
		}
		if err = store.New(db).InsertRunOCRSelection(ctx, store.InsertRunOCRSelectionParams{RunID: runID, ItemKey: key, Gap: gap}); err != nil {
			t.Fatal(err)
		}
	}
	return env, hashes
}

func TestVision_SelectionQueriesAndCheckEngineOrder(t *testing.T) {
	ctx := context.Background()
	env, hashes := visionRunFixture(t)
	source := ocrstore.Open(env.root, env.db)
	for _, item := range []struct {
		index          int
		engine, origin string
	}{{0, "vision-ocr/builtin-latest", "banhmi"}, {1, ocrvision.EngineID, "vision"}, {4, "other-engine", "vision"}} {
		if _, err := source.Put(ctx, hashes[item.index], item.engine, item.origin, pgtype.Int4{Int32: 1, Valid: true}, []byte("text")); err != nil {
			t.Fatal(err)
		}
	}
	keys, err := store.New(env.db).ListVisionSelectionKeys(ctx, env.run.ID)
	if err != nil || strings.Join(keys, ",") != "vn:vbpl:1,vn:vbpl:2,vn:vbpl:3,vn:vbpl:5" {
		t.Fatalf("keys=%v err=%v", keys, err)
	}
	docs, err := env.visionDocuments(ctx, "")
	if err != nil || len(docs) != 2 {
		t.Fatalf("default=%+v err=%v", docs, err)
	}
	selected := map[string]bool{}
	for _, doc := range docs {
		selected[doc.Hash] = true
	}
	if !selected[hashes[2]] || !selected[hashes[4]] {
		t.Fatalf("selected=%v", selected)
	}
	explicit, err := env.visionDocuments(ctx, hashes[0])
	if err != nil || len(explicit) != 1 || explicit[0].Present {
		t.Fatalf("explicit=%+v err=%v", explicit, err)
	}
	if _, err = source.Put(ctx, hashes[0], ocrvision.EngineID, "vision", pgtype.Int4{Int32: 1, Valid: true}, []byte("fresh comparison")); err != nil {
		t.Fatal(err)
	}
	explicit, err = env.visionDocuments(ctx, hashes[0])
	if err != nil || len(explicit) != 1 || !explicit[0].Present {
		t.Fatal("fresh comparison not present")
	}
	checked, err := env.documents(ctx, "checker")
	if err != nil || len(checked) != 2 {
		t.Fatalf("check=%+v err=%v", checked, err)
	}
	engines := map[string]string{}
	for _, doc := range checked {
		engines[doc.Entry.PdfSha256] = doc.Entry.EngineID
	}
	if engines[hashes[0]] != "vision-ocr/builtin-latest" || engines[hashes[1]] != ocrvision.EngineID {
		t.Fatalf("engines=%v", engines)
	}
	if _, err = env.visionDocuments(ctx, strings.Repeat("f", 64)); err == nil {
		t.Fatal("selected PDF outside run")
	}
	explicit, err = env.visionDocuments(ctx, hashes[3])
	if err != nil || len(explicit) != 1 {
		t.Fatal("explicit PDF required needs_ocr gap")
	}
}

func TestVision_CommandDryRunWithoutCredentials(t *testing.T) {
	env, _ := visionRunFixture(t)
	t.Setenv("CORPUS_DATABASE_URL", env.db.Config().ConnString())
	t.Setenv("CORPUS_ARTIFACTS", env.artifacts.Root)
	t.Setenv("CORPUS_OCR_STORE", env.root)
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/nonexistent/credentials")
	var out, stderr bytes.Buffer
	status := Run(context.Background(), []string{"ocr", "vision", "vn", "20990101", "--max-pages", "1", "--dry-run"}, &out, &stderr)
	if status != 0 || !strings.Contains(out.String(), "documents=4 pages=4 failed=0\n") {
		t.Fatalf("status=%d output=%q error=%s", status, out.String(), stderr.String())
	}
}

func visionTestPDF(n int) []byte {
	var pdf bytes.Buffer
	fmt.Fprintf(&pdf, "%%PDF-1.7\n%% fixture %d\n", n)
	offsets := []int{0}
	for i, obj := range []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 72 72] /Resources << >> >>"} {
		offsets = append(offsets, pdf.Len())
		fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xref := pdf.Len()
	pdf.WriteString("xref\n0 4\n0000000000 65535 f \n")
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size 4 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xref)
	return pdf.Bytes()
}
