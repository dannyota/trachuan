package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/config"
	"danny.vn/trachuan/apps/corpus/internal/ocrcheck"
	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/extract"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sys/unix"
)

type checkRun struct {
	db        *pgxpool.Pool
	root      string
	artifacts artifact.Store
	run       stage.Run
}

func openCheckRun(ctx context.Context, cc, rel string) (*checkRun, error) {
	root := os.Getenv("CORPUS_OCR_STORE")
	if root == "" {
		return nil, fmt.Errorf("CORPUS_OCR_STORE is required")
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return nil, err
	}
	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("open workshop database: %w", err)
	}
	row, err := store.New(db).GetRun(ctx, store.GetRunParams{Country: cc, Rel: rel})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("read OCR run: %w", err)
	}
	var snapshot stage.Snapshot
	if err = json.Unmarshal(row.Snapshot, &snapshot); err != nil {
		db.Close()
		return nil, fmt.Errorf("decode run snapshot: %w", err)
	}
	return &checkRun{db: db, root: root, artifacts: artifact.Store{Root: cfg.Artifacts}, run: stage.Run{ID: row.ID, Country: cc, Rel: rel, Snapshot: snapshot}}, nil
}
func (e *checkRun) checkDocuments(ctx context.Context, checker string, allEntries bool) ([]ocrcheck.Document, error) {
	q := store.New(e.db)
	active, err := membership.FinalActiveKeys(ctx, q, e.run)
	if err != nil {
		return nil, err
	}
	inputs, err := q.ListExtractInputs(ctx, e.run.ID)
	if err != nil {
		return nil, fmt.Errorf("list check inputs: %w", err)
	}
	source := ocrstore.Open(e.root, e.db)
	var docs []ocrcheck.Document
	for _, input := range inputs {
		if active != nil && !active[input.ItemKey] {
			continue
		}
		item, getErr := q.GetExtractInput(ctx, store.GetExtractInputParams{RunID: e.run.ID, ItemKey: input.ItemKey})
		if getErr != nil {
			return nil, fmt.Errorf("load check input: %w", getErr)
		}
		dir := e.artifacts.Abs(item.Path)
		hash, verifyErr := verifyCheckBundle(dir, input.ItemKey)
		if verifyErr != nil {
			return nil, fmt.Errorf("verify check bundle: %w", verifyErr)
		}
		if hash != item.Sha256 || hash != input.Sha256 {
			return nil, fmt.Errorf("check bundle hash mismatch")
		}
		var selected []extract.CheckSource
		var selectErr error
		if allEntries {
			selected, selectErr = extract.SelectCheckSources(ctx, dir, input.ItemKey, source, extract.AcceptedEngines(), checker)
		} else {
			var one *extract.CheckSource
			one, selectErr = extract.SelectCheckSource(ctx, dir, input.ItemKey, source, extract.AcceptedEngines(), checker)
			if one != nil {
				selected = append(selected, *one)
			}
		}
		if selectErr != nil {
			return nil, fmt.Errorf("select OCR check source: %w", selectErr)
		}
		for _, chosen := range selected {
			entry := chosen.Entry
			docs = append(docs, ocrcheck.Document{Key: input.ItemKey, PDFName: chosen.PDFName, Entry: entry, PDFPath: filepath.Join(dir, "files", entry.PdfSha256), ReadText: func() ([]byte, error) { return source.ReadText(entry) }})
		}
	}
	return docs, nil
}
func lockOCRStore(root string) (func(), error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, fmt.Errorf("create OCR store directory: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(root, "check.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open OCR check lock: %w", err)
	}
	if err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("another OCR check command holds this store: %w", err)
	}
	return func() { _ = f.Close() }, nil
}
