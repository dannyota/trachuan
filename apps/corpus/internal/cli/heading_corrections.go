package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/config"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/extract"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func runHeadingCorrections(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		_, _ = fmt.Fprintln(stderr, "usage: corpus heading-corrections <cc> <rel> [--corrections FILE]")
		return 2
	}
	opts, err := options(args[2:], map[string]bool{"--corrections": true})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	path := opts["--corrections"]
	if path == "" {
		path = "sources/" + args[0] + "-corrections.yaml"
	}
	checks, err := headingCorrections(ctx, args[0], args[1], path)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return writeHeadingCorrections(stdout, stderr, checks)
}

func headingCorrections(ctx context.Context, cc, rel, path string) ([]extract.HeadingCorrectionCheck, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		data, err = []byte("[]"), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read corrections: %w", err)
	}
	entries, err := source.ParseCorrections(data)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return nil, err
	}
	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("open workshop database: %w", err)
	}
	defer db.Close()
	q := store.New(db)
	row, err := q.GetRun(ctx, store.GetRunParams{Country: cc, Rel: rel})
	if err != nil {
		return nil, fmt.Errorf("read correction-check run: %w", err)
	}
	var snapshot stage.Snapshot
	if err = json.Unmarshal(row.Snapshot, &snapshot); err != nil {
		return nil, fmt.Errorf("decode run snapshot: %w", err)
	}
	run := stage.Run{ID: row.ID, Country: cc, Rel: rel, Snapshot: snapshot}
	active, err := membership.FinalActiveKeys(ctx, q, run)
	if err != nil {
		return nil, err
	}
	inputs, err := q.ListExtractInputs(ctx, run.ID)
	if err != nil {
		return nil, fmt.Errorf("list correction-check inputs: %w", err)
	}
	wanted := make(map[string]bool)
	for _, entry := range entries {
		key := entry.Evidence.Key
		if key == "" {
			key = entry.Key
		}
		wanted[key] = true
	}
	artifacts := artifact.Store{Root: cfg.Artifacts}
	bundles := make(map[string]string)
	for _, input := range inputs {
		if !wanted[input.ItemKey] || (active != nil && !active[input.ItemKey]) {
			continue
		}
		item, getErr := q.GetExtractInput(ctx, store.GetExtractInputParams{RunID: run.ID, ItemKey: input.ItemKey})
		if getErr != nil {
			return nil, fmt.Errorf("load correction-check bundle: %w", getErr)
		}
		dir := artifacts.Abs(item.Path)
		hash, verifyErr := artifact.Verify(dir)
		if verifyErr != nil {
			return nil, fmt.Errorf("verify correction-check bundle: %w", verifyErr)
		}
		if hash != item.Sha256 || hash != input.Sha256 {
			return nil, fmt.Errorf("correction-check bundle hash mismatch")
		}
		bundles[input.ItemKey] = dir
	}
	return extract.CheckHeadingCorrections(ctx, entries, bundles)
}

func writeHeadingCorrections(stdout, stderr io.Writer, checks []extract.HeadingCorrectionCheck) int {
	if err := json.NewEncoder(stdout).Encode(checks); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	missing := 0
	for _, check := range checks {
		if !check.QuoteFound {
			missing++
		}
	}
	if missing > 0 {
		_, _ = fmt.Fprintf(stderr, "%d correction evidence quotes missing; update corrections before the run\n", missing)
		return 1
	}
	return 0
}
