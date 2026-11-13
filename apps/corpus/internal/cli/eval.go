package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/config"
	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/eval"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/worker"
	"danny.vn/trachuan/internal/retrieval"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sys/unix"
)

func evalExit(r evaluation.Report, require bool) int {
	if r.Status != "complete" {
		return 1
	}
	if require {
		if !r.QualityPass || !passingEvalFloors(r.Floors, false) {
			return 2
		}
		development := r.Cohorts["development"]
		if !passingEvalFloors(development.Floors, development.CaseCount == 0) {
			return 2
		}
		for _, rejection := range r.Rejections {
			if !rejection.Passed {
				return 2
			}
		}
	}

	return 0
}
func passingEvalFloors(floors []evaluation.Floor, allowNotRun bool) bool {
	if allowNotRun {
		notRun := true
		for _, f := range floors {
			notRun = notRun && f.Status == "not_run"
		}
		if notRun {
			return true
		}
	}
	if len(floors) != 2 {
		return false
	}
	seen := make(map[string]bool)
	for _, f := range floors {
		if f.Status != "pass" || f.Arms != "hybrid" || f.Limit != 8 || (f.Mode != "exact" && f.Mode != "ann") || f.Name != "current_law_precision" {
			return false
		}
		key := string(f.Mode) + ":" + f.Name
		if seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}

func runEval(ctx context.Context, args []string, _ io.Writer, stderr io.Writer) int {
	return runEvalWith(ctx, args, stderr, prepareEval, readEvalRecipeVersion)
}

type evalRunner interface {
	Evaluate(context.Context, stage.Env, stage.Run, evaluation.Options) (evaluation.Report, evaluation.Cache, error)
}

type evalTarget struct {
	runner evalRunner
	env    stage.Env
	run    stage.Run
	close  func()
}

func runEvalWith(ctx context.Context, args []string, stderr io.Writer, prepare func(context.Context, string, string, string) (evalTarget, error), versions ...func(context.Context, string, string) (int, error)) int {
	fail := func(err error) int { _, _ = fmt.Fprintln(stderr, err); return 1 }
	if len(args) < 2 {
		return fail(fmt.Errorf("usage: corpus eval <cc> <rel> --out DIR [--previous FILE] [--require-floors] [--cap N|none] [--article-cap N|none] [--merge-siblings] [--historical-policy agent] [--non-current-tail N] [--not-yet-eligible]"))
	}
	opts, err := options(args[2:], map[string]bool{"--out": true, "--previous": true, "--require-floors": false, "--cap": true, "--article-cap": true, "--merge-siblings": false, "--historical-policy": true, "--non-current-tail": true, "--not-yet-eligible": false})
	if err != nil {
		return fail(err)
	}
	if opts["--merge-siblings"] == "true" {
		if len(versions) != 1 {
			return fail(fmt.Errorf("--merge-siblings: release recipe lookup unavailable"))
		}
		version, readErr := versions[0](ctx, args[0], args[1])
		if readErr != nil {
			return fail(readErr)
		}
		return fail(fmt.Errorf("--merge-siblings is no longer an override; release %s/%s uses retrieval recipe %d, and recipe 3 merges siblings by default", args[0], args[1], version))
	}
	if opts["--out"] == "" {
		return fail(fmt.Errorf("--out DIR is required"))
	}
	var capOverride *retrieval.CapOverride
	if value, ok := opts["--cap"]; ok {
		capOverride, err = retrieval.ParseCapOverride(value)
		if err != nil {
			return fail(fmt.Errorf("--cap: %w", err))
		}
	}
	var articleCapOverride *retrieval.ArticleCapOverride
	if value, ok := opts["--article-cap"]; ok {
		articleCapOverride, err = retrieval.ParseArticleCapOverride(value)
		if err != nil {
			return fail(fmt.Errorf("--article-cap: %w", err))
		}
	}
	validity := evaluation.ValidityOverrides{NotYetEligible: opts["--not-yet-eligible"] == "true"}
	if value, ok := opts["--historical-policy"]; ok {
		if value != "agent" {
			return fail(fmt.Errorf("invalid historical policy: expected agent"))
		}
		validity.HistoricalPolicy = &value
	}
	if value, ok := opts["--non-current-tail"]; ok {
		n, parseErr := strconv.Atoi(value)
		if parseErr != nil || n < 0 || n > 5 || strconv.Itoa(n) != value {
			return fail(fmt.Errorf("invalid non-current tail: expected 0 to 5"))
		}
		validity.NonCurrentTail = &n
	}
	if err = checkEvaluationOutput(opts["--out"]); err != nil {
		return fail(err)
	}
	target, err := prepare(ctx, args[0], args[1], opts["--previous"])
	if err != nil {
		return fail(err)
	}
	defer target.close()
	report, cache, err := target.runner.Evaluate(ctx, target.env, target.run, evaluation.Options{CapOverride: capOverride, ArticleCapOverride: articleCapOverride, ValidityOverrides: validity})
	if err != nil {
		return fail(err)
	}
	if err = writeEvaluationOutput(opts["--out"], report, cache); err != nil {
		return fail(err)
	}
	return evalExit(report, opts["--require-floors"] == "true")
}

func prepareEval(ctx context.Context, cc, rel, previous string) (evalTarget, error) {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return evalTarget{}, err
	}
	deps, err := worker.IndexDeps(ctx, cfg)
	if err != nil {
		return evalTarget{}, err
	}
	deps.Eval.Previous = previous
	db, err := openEvalDB(ctx, cc, rel, cfg.DatabaseURL)
	if err != nil {
		return evalTarget{}, err
	}
	row, err := store.New(db).GetRun(ctx, store.GetRunParams{Country: cc, Rel: rel})
	if err != nil {
		db.Close()
		return evalTarget{}, fmt.Errorf("corpus eval %s/%s get run: %w", cc, rel, err)
	}
	var snapshot stage.Snapshot
	if err = json.Unmarshal(row.Snapshot, &snapshot); err != nil {
		db.Close()
		return evalTarget{}, fmt.Errorf("corpus eval %s/%s decode snapshot: %w", cc, rel, err)
	}
	run := stage.Run{ID: row.ID, Country: row.Country, Rel: row.Rel, Generation: int(row.Generation), Snapshot: snapshot}
	return evalTarget{runner: eval.New(deps.Eval), env: stage.Env{DB: db, Artifacts: artifact.Store{Root: cfg.Artifacts}}, run: run, close: db.Close}, nil
}

type evaluationWriter interface{ WriteFiles(string) error }

func checkEvaluationOutput(out string) error {
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
	return nil
}

func writeEvaluationOutput(out string, report, cache evaluationWriter) error {
	out = filepath.Clean(out)
	if err := checkEvaluationOutput(out); err != nil {
		return err
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
	if err = unix.Rmdir(out); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove empty output: %w", err)
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
