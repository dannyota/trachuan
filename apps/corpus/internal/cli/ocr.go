package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/config"
	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func runOCR(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "vision":
			return runOCRVision(ctx, args[1:], stdout, stderr)
		case "check":
			return runOCRCheck(ctx, args[1:], stdout, stderr)
		case "review":
			return runOCRReview(ctx, args[1:], stdout, stderr)
		}
	}
	if len(args) != 4 || args[0] != "import-banhmi" {
		_, _ = fmt.Fprintln(stderr, "usage: corpus ocr <check|review|vision|import-banhmi> <cc> <rel> [options]")
		return 2
	}
	root := os.Getenv("CORPUS_OCR_STORE")
	if root == "" {
		_, _ = fmt.Fprintln(stderr, "CORPUS_OCR_STORE is required")
		return 1
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer db.Close()
	q := store.New(db)
	run, err := q.GetRun(ctx, store.GetRunParams{Country: args[1], Rel: args[2]})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "load run:", err)
		return 1
	}
	pdfs, err := importPDFs(ctx, q, artifact.Store{Root: cfg.Artifacts}, run.ID)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	counts, err := importBanhmi(ctx, ocrstore.Open(root, db), args[3], pdfs)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "imported: %d\nalready present: %d\nnot in the run: %d\nrejected: %d\n", counts.imported, counts.present, counts.notInRun, counts.rejected)
	return 0
}

func importPDFs(ctx context.Context, q *store.Queries, artifacts artifact.Store, runID int64) (map[string]bool, error) {
	inputs, err := q.ListExtractInputs(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("list fetch items: %w", err)
	}
	pdfs := make(map[string]bool)
	for _, input := range inputs {
		item, err := q.GetExtractInput(ctx, store.GetExtractInputParams{RunID: runID, ItemKey: input.ItemKey})
		if err != nil {
			return nil, fmt.Errorf("load fetch item: %w", err)
		}
		dir := artifacts.Abs(item.Path)
		hash, err := artifact.Verify(dir)
		if err != nil {
			return nil, fmt.Errorf("verify fetch artifact: %w", err)
		}
		if hash != item.Sha256 || hash != input.Sha256 {
			return nil, fmt.Errorf("fetch artifact hash mismatch")
		}
		doc, err := fetch.ReadDocument(dir, input.ItemKey)
		if err != nil {
			return nil, fmt.Errorf("read fetch bundle: %w", err)
		}
		for _, f := range doc.Files {
			if strings.EqualFold(filepath.Ext(f.Name), ".pdf") {
				pdfs[f.SHA256] = true
			}
		}
	}
	return pdfs, nil
}
