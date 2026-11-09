package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

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
		if len(r.Floors) != 8 {
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
	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fail(err)
	}
	defer db.Close()
	row, err := store.New(db).GetRun(ctx, store.GetRunParams{Country: args[0], Rel: args[1]})
	if err != nil {
		return fail(err)
	}
	var snapshot stage.Snapshot
	if err = json.Unmarshal(row.Snapshot, &snapshot); err != nil {
		return fail(err)
	}
	run := stage.Run{ID: row.ID, Country: row.Country, Rel: row.Rel, Generation: int(row.Generation), Snapshot: snapshot}
	report, cache, err := eval.New(deps.Eval).Evaluate(ctx, stage.Env{DB: db, Artifacts: artifact.Store{Root: cfg.Artifacts}}, run)
	if err != nil {
		return fail(err)
	}
	if err = report.WriteFiles(opts["--out"]); err != nil {
		return fail(err)
	}
	if err = cache.WriteFiles(opts["--out"]); err != nil {
		return fail(err)
	}
	return evalExit(report, opts["--require-floors"] == "true")
}
