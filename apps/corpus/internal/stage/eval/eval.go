// Package eval adapts the evaluation harness to the pipeline.
package eval

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/internal/retrieval"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Config supplies the build and evaluation dependencies.
type Config struct {
	Build    build.Config
	Suite    evaluation.Suite
	Embedder retrieval.Embedder
	Previous string
}

// Stage evaluates one built release.
type Stage struct {
	cfg    Config
	lookup func(context.Context, stage.Env, stage.Run) (artifact.Result, error)
	ensure func(context.Context, stage.Env, stage.Run) (build.Database, error)
}

// New constructs an evaluation stage.
func New(cfg Config) *Stage { return &Stage{cfg: cfg} }

// Name returns the stage name.
func (*Stage) Name() string { return "eval" }

// Recipe identifies the scoring and cache policies.
func (s *Stage) Recipe(stage.Snapshot) string {
	return stage.RecipeHash("eval", "agent-eval/1", "legacy-matcher/1", "product-matcher/1", "recall,mrr,current-law-precision,abstention,evidence,ann/1", strings.Join(s.cfg.Suite.Rejections, "\x00"), "diacritics_required:Vui lòng nhập câu hỏi tiếng Việt có dấu hoặc số hiệu văn bản.", "cache/1:200:100", "nearest-rank", "0.90:0.66:0.99:0.95")
}

func dateForRun(run stage.Run) (time.Time, error) {
	if len(run.Rel) < 8 {
		return time.Time{}, fmt.Errorf("invalid release date")
	}
	d, err := time.Parse("20060102", run.Rel[:8])
	if err != nil {
		return time.Time{}, fmt.Errorf("parse release date: %w", err)
	}
	return d, nil
}
func (s *Stage) inputHash(art artifact.Result, run stage.Run) (string, error) {
	d, err := dateForRun(run)
	if err != nil {
		return "", err
	}
	previous := ""
	if s.cfg.Previous != "" {
		b, e := os.ReadFile(s.cfg.Previous)
		if e != nil {
			return "", fmt.Errorf("read previous report: %w", e)
		}
		previous = fmt.Sprintf("%x", sha256.Sum256(b))
	}
	h := s.cfg.Suite.Hashes
	return stage.RecipeHash("eval-input", "1", art.SHA256, h.Legacy, h.OldProduct, h.IdentityMap, h.Inventory, previous, d.Format("2006-01-02"), "exact,ann"), nil
}

// Plan resolves the build and selects its single evaluation item.
func (s *Stage) Plan(ctx context.Context, env stage.Env, run stage.Run) ([]stage.Item, error) {
	art, err := s.lookupBuild(ctx, env, run)
	if err != nil {
		return nil, err
	}
	hash, err := s.inputHash(art, run)
	if err != nil {
		return nil, err
	}
	return []stage.Item{{Key: run.Country + "/" + run.Rel, InputHash: hash}}, nil
}

// Evaluate resolves the build and runs a diagnostic evaluation.
func (s *Stage) Evaluate(ctx context.Context, env stage.Env, run stage.Run) (evaluation.Report, evaluation.Cache, error) {
	db, err := s.ensureBuild(ctx, env, run)
	if err != nil {
		return evaluation.Report{}, evaluation.Cache{}, fmt.Errorf("ensure build: %w", err)
	}
	art, err := s.lookupBuild(ctx, env, run)
	if err != nil {
		return evaluation.Report{}, evaluation.Cache{}, err
	}
	date, err := dateForRun(run)
	if err != nil {
		return evaluation.Report{}, evaluation.Cache{}, err
	}
	pool, err := pgxpool.New(ctx, db.URL)
	if err != nil {
		return evaluation.Report{}, evaluation.Cache{}, fmt.Errorf("open corpus database: %w", err)
	}
	defer pool.Close()
	engine, err := retrieval.Open(ctx, pool, s.cfg.Embedder, retrieval.Options{Profile: "evaluation", Now: func() time.Time { return date }})
	if err != nil {
		return evaluation.Report{}, evaluation.Cache{}, fmt.Errorf("open retrieval: %w", err)
	}
	calibration, err := os.ReadFile(filepath.Join(env.Artifacts.Abs(art.Path), "calibration.json"))
	if err != nil {
		return evaluation.Report{}, evaluation.Cache{}, fmt.Errorf("read calibration: %w", err)
	}
	var cal any
	if err = json.Unmarshal(calibration, &cal); err != nil {
		return evaluation.Report{}, evaluation.Cache{}, fmt.Errorf("decode calibration: %w", err)
	}
	report, cache, err := evaluation.Run(ctx, engine, s.cfg.Suite, evaluation.Options{EvaluationDate: retrieval.Date{Year: date.Year(), Month: date.Month(), Day: date.Day()}, PreviousReport: s.cfg.Previous, BuildArtifactSHA256: art.SHA256, DumpSHA256: db.Record.Dump.SHA256, DatabaseInstanceKind: db.Instance, Calibration: cal, Software: map[string]string{"postgresql": db.Record.PostgresVersion, "pgvector": db.Record.PGVectorVersion, "ltree": db.Record.LtreeVersion, "pg_dump": db.Record.PgDumpVersion, "worker": db.Record.WorkerVersion}})
	if err != nil {
		return report, cache, fmt.Errorf("run evaluation: %w", err)
	}
	return report, cache, nil
}

// Process publishes the report and caches for one evaluation.
func (s *Stage) Process(ctx context.Context, env stage.Env, run stage.Run, item stage.Item) (stage.Output, error) {
	ctx, cancel := context.WithTimeout(ctx, 50*time.Minute)
	defer cancel()
	if item.Key != run.Country+"/"+run.Rel {
		return stage.Output{}, fmt.Errorf("evaluation item identity mismatch")
	}
	art, err := s.lookupBuild(ctx, env, run)
	if err != nil {
		return stage.Output{}, err
	}
	if err = s.checkInputHash(art, run, item.InputHash); err != nil {
		return stage.Output{}, err
	}
	report, cache, err := s.Evaluate(ctx, env, run)
	if err != nil {
		return stage.Output{}, err
	}
	result, err := env.Artifacts.Publish(ctx, s.Name(), item.InputHash, s.Recipe(run.Snapshot), func(dir string) error {
		if e := report.WriteFiles(dir); e != nil {
			return e
		}
		return cache.WriteFiles(dir)
	})
	if err != nil {
		return stage.Output{}, fmt.Errorf("publish evaluation: %w", err)
	}
	return stage.Output{Artifact: &result, State: "done"}, nil
}

// Rows provides no workshop projections.
func (*Stage) Rows(context.Context, stage.Env, artifact.Result) (func(context.Context, pgx.Tx, int64) error, error) {
	return nil, nil
}

// Resolve validates the report and restores its run-specific gaps.
func (s *Stage) Resolve(_ context.Context, env stage.Env, run stage.Run, item stage.Item, art artifact.Result) (stage.Resolution, error) {
	if item.Key != run.Country+"/"+run.Rel || item.InputHash != art.InputHash {
		return stage.Resolution{}, fmt.Errorf("evaluation item identity mismatch")
	}
	sum, err := artifact.Verify(env.Artifacts.Abs(art.Path))
	if err != nil || sum != art.SHA256 {
		return stage.Resolution{}, fmt.Errorf("invalid evaluation artifact")
	}
	data, err := os.ReadFile(filepath.Join(env.Artifacts.Abs(art.Path), "report.json"))
	if err != nil {
		return stage.Resolution{}, fmt.Errorf("read report: %w", err)
	}
	var r evaluation.Report
	if err = json.Unmarshal(data, &r); err != nil {
		return stage.Resolution{}, fmt.Errorf("decode report: %w", err)
	}
	if r.Status != "complete" || r.Country != run.Country || r.Rel != run.Rel || r.ReleaseEligible {
		return stage.Resolution{}, fmt.Errorf("invalid evaluation report")
	}
	return stage.Resolution{Gaps: reportGaps(r)}, nil
}
func reportGaps(r evaluation.Report) []stage.Gap {
	var gaps []stage.Gap
	for _, f := range r.Floors {
		if f.Status == "fail" || f.Status == "unavailable" {
			gaps = append(gaps, stage.Gap{Kind: "eval_floor_failed", Detail: strings.Join([]string{string(f.Mode), f.Name}, ":")})
		}
	}
	for _, v := range r.Rejections {
		if !v.Passed {
			gaps = append(gaps, stage.Gap{Kind: "eval_rejection_failed", Detail: v.CaseID})
		}
	}
	return gaps
}

func (s *Stage) checkInputHash(art artifact.Result, run stage.Run, expected string) error {
	hash, err := s.inputHash(art, run)
	if err != nil {
		return fmt.Errorf("compute evaluation input hash: %w", err)
	}
	if hash != expected {
		return fmt.Errorf("evaluation input mismatch")
	}
	return nil
}

func (s *Stage) lookupBuild(ctx context.Context, env stage.Env, run stage.Run) (artifact.Result, error) {
	if s.lookup != nil {
		return s.lookup(ctx, env, run)
	}
	return build.VerifyForRun(ctx, s.cfg.Build, store.New(env.DB), env.Artifacts, run)
}
func (s *Stage) ensureBuild(ctx context.Context, env stage.Env, run stage.Run) (build.Database, error) {
	if s.ensure != nil {
		return s.ensure(ctx, env, run)
	}
	return build.EnsureForRun(ctx, s.cfg.Build, store.New(env.DB), env.Artifacts, run)
}
