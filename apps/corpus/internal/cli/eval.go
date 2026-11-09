package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/config"
	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/eval"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/worker"
	"github.com/jackc/pgx/v5/pgxpool"
)

func evalExit(r evaluation.Report, require bool) int {
	if r.Status != "complete" {
		return 1
	}
	if require {
		if !r.QualityPass || len(r.Floors) != 8 {
			return 2
		}
		for _, f := range r.Floors {
			if f.Status != "pass" {
				return 2
			}
		}
	}
	return 0
}
func runEval(ctx context.Context, args []string, _ io.Writer, stderr io.Writer) int {
	fail := func(err error) int { _, _ = fmt.Fprintln(stderr, err); return 1 }
	if len(args) < 2 {
		return fail(fmt.Errorf("usage: corpus eval <cc> <rel> --out DIR [--previous FILE] [--require-floors]"))
	}
	opts, err := options(args[2:], map[string]bool{"--out": true, "--previous": true, "--require-floors": false})
	if err != nil {
		return fail(err)
	}
	if opts["--out"] == "" {
		return fail(fmt.Errorf("--out DIR is required"))
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return fail(err)
	}
	deps, err := worker.IndexDeps(ctx, cfg)
	if err != nil {
		return fail(err)
	}
	deps.Eval.Previous = opts["--previous"]
	db, err := openEvalDB(ctx, args[0], args[1], cfg.DatabaseURL)
	if err != nil {
		return fail(err)
	}
	defer db.Close()
	row, err := store.New(db).GetRun(ctx, store.GetRunParams{Country: args[0], Rel: args[1]})
	if err != nil {
		return fail(fmt.Errorf("corpus eval %s/%s get run: %w", args[0], args[1], err))
	}
	var snapshot stage.Snapshot
	if err = json.Unmarshal(row.Snapshot, &snapshot); err != nil {
		return fail(fmt.Errorf("corpus eval %s/%s decode snapshot: %w", args[0], args[1], err))
	}
	run := stage.Run{ID: row.ID, Country: row.Country, Rel: row.Rel, Generation: int(row.Generation), Snapshot: snapshot}
	report, cache, err := eval.New(deps.Eval).Evaluate(ctx, stage.Env{DB: db, Artifacts: artifact.Store{Root: cfg.Artifacts}}, run)
	if err != nil {
		return fail(err)
	}
	if err = writeEvaluationOutput(opts["--out"], report, cache); err != nil {
		return fail(err)
	}
	return evalExit(report, opts["--require-floors"] == "true")
}

type evaluationWriter interface{ WriteFiles(string) error }

func writeEvaluationOutput(out string, report, cache evaluationWriter) error {
	out = filepath.Clean(out)
	if info, err := os.Lstat(out); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("output is not a directory")
		}
		entries, readErr := os.ReadDir(out)
		if readErr != nil {
			return fmt.Errorf("inspect output: %w", readErr)
		}
		if len(entries) != 0 {
			return fmt.Errorf("output directory is not empty")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect output: %w", err)
	}
	parent := filepath.Dir(out)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return fmt.Errorf("create output parent: %w", err)
	}
	temp, err := os.MkdirTemp(parent, ".eval-")
	if err != nil {
		return fmt.Errorf("create temporary output: %w", err)
	}
	defer func() { _ = os.RemoveAll(temp) }()
	store := artifact.Store{Root: temp}
	hash := strings.Repeat("0", 64)
	result, err := store.Publish(context.Background(), "eval", hash, hash, func(dir string) error {
		if writeErr := report.WriteFiles(dir); writeErr != nil {
			return writeErr
		}
		return cache.WriteFiles(dir)
	})
	if err != nil {
		return fmt.Errorf("write evaluation output: %w", err)
	}
	if err = os.Rename(store.Abs(result.Path), out); err != nil {
		return fmt.Errorf("publish evaluation output: %w", err)
	}
	return nil
}

func openEvalDB(ctx context.Context, cc, rel, url string) (*pgxpool.Pool, error) {
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("corpus eval %s/%s open workshop: %w", cc, rel, err)
	}
	return db, nil
}
