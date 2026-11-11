package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"danny.vn/trachuan/apps/server/internal/admin"
	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

type reportExporter interface {
	createReportsBatch(context.Context, string) error
	reportsSnapshot(context.Context, string) (store.BatchSnapshotRow, error)
}

func (s credentialStore) createReportsBatch(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin export: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.WithTx(tx)
	if err = q.LockReports(ctx); err != nil {
		return fmt.Errorf("lock reports: %w", err)
	}
	count, err := q.CountRetainedReports(ctx)
	if err != nil {
		return fmt.Errorf("count reports: %w", err)
	}
	if count >= 100000 {
		return errors.New("reports retained cap reached")
	}
	n, err := q.CreateExportBatch(ctx, id)
	if err != nil {
		return fmt.Errorf("create batch: %w", err)
	}
	if n == 0 {
		return errors.New("batch unavailable")
	}
	if count+n > 100000 {
		return errors.New("reports retained cap reached")
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit export: %w", err)
	}
	return nil
}
func (s credentialStore) reportsSnapshot(ctx context.Context, id string) (store.BatchSnapshotRow, error) {
	return s.BatchSnapshot(ctx, store.BatchSnapshotParams{BatchID: id, Before: time.Now().Add(-90 * 24 * time.Hour)})
}
func exportReports(ctx context.Context, base admin.Store, args []string, out io.Writer) error {
	f := flag.NewFlagSet("export", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	dir := f.String("out", "", "output directory")
	batch := f.String("batch", "", "batch ID")
	if f.Parse(args) != nil || f.NArg() != 0 || *dir == "" {
		return errors.New("invalid export flags")
	}
	s, ok := base.(reportExporter)
	if !ok {
		return errors.New("reports store unavailable")
	}
	if *batch == "" {
		var b [16]byte
		if _, err := rand.Read(b[:]); err != nil {
			return fmt.Errorf("create batch id: %w", err)
		}
		*batch = hex.EncodeToString(b[:])
		if _, err := fmt.Fprintln(out, *batch); err != nil {
			return err
		}
		if err := s.createReportsBatch(ctx, *batch); err != nil {
			return err
		}
	}
	if len(*batch) > 128 || strings.Trim(*batch, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_") != "" {
		return errors.New("invalid batch id")
	}
	snapshot, err := s.reportsSnapshot(ctx, *batch)
	if err != nil {
		return fmt.Errorf("read batch: %w", err)
	}
	if snapshot.Expired {
		return errors.New("batch expired")
	}
	var rows []exportReport
	if err = json.Unmarshal(snapshot.Reports, &rows); err != nil || len(rows) == 0 {
		return errors.New("batch unavailable")
	}
	if err = os.MkdirAll(*dir, 0700); err != nil {
		return fmt.Errorf("create export directory: %w", err)
	}
	target := filepath.Join(*dir, *batch)
	if _, err = os.Lstat(target); err == nil {
		return errors.New("batch directory exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("check batch directory: %w", err)
	}
	tmp, err := os.MkdirTemp(*dir, ".reports-")
	if err != nil {
		return fmt.Errorf("create temporary directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err = writeReportExport(tmp, *batch, rows); err != nil {
		return err
	}
	if err = renameReportDirectory(tmp, target); err != nil {
		if errors.Is(err, os.ErrExist) {
			return errors.New("batch directory exists")
		}
		return fmt.Errorf("publish batch: %w", err)
	}
	return nil
}
