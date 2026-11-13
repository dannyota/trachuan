package eval

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"danny.vn/trachuan/internal/retrieval"
)

func TestEvalStage_ReportStatusAndGaps(t *testing.T) {
	r := evaluation.Report{Status: "complete", Floors: []evaluation.Floor{{Metric: evaluation.Metric{Mode: retrieval.Mode("exact"), Name: "recall"}, Status: "fail"}, {Metric: evaluation.Metric{Mode: retrieval.Mode("ann"), Name: "mrr"}, Status: "unavailable"}, {Status: "pass"}}, Rejections: []evaluation.Rejection{{CaseID: "bad", Passed: false}, {CaseID: "ok", Passed: true}}}
	gaps := reportGaps(r)
	if len(gaps) != 3 || gaps[0].Kind != "eval_floor_failed" || gaps[0].Detail != "exact:recall" || gaps[1].Detail != "ann:mrr" || gaps[2].Kind != "eval_rejection_failed" || gaps[2].Detail != "bad" {
		t.Fatalf("gaps: %+v", gaps)
	}
}

func TestInputHash_OmitsInventory(t *testing.T) {
	s := New(Config{Suite: evaluation.Suite{Hashes: evaluation.FixtureHashes{Legacy: "legacy", OldProduct: "product", IdentityMap: "map", ValidityCases: "validity"}}})
	got, err := s.inputHash(artifact.Result{SHA256: "build"}, stage.Run{Rel: "20990101"})
	if err != nil {
		t.Fatal(err)
	}
	want := stage.RecipeHash("eval-input", "2", "build", "legacy", "product", "map", "validity", "", "", "2099-01-01", "exact,ann")
	if got != want {
		t.Fatalf("input hash = %s, want %s", got, want)
	}
}

func TestRecipe_IncludesCompact(t *testing.T) {
	s := New(Config{})
	want := stage.RecipeHash("eval", evaluation.ScorerVersion, "legacy-matcher/2", "product-matcher/1", "recall,mrr,current-law-precision,abstention,evidence,ann/1", "", "diacritics_required:Vui lòng nhập câu hỏi tiếng Việt có dấu hoặc số hiệu văn bản.", "cache/1:200:100", "nearest-rank", "gates:0.99;targets:recall20=0.90,mrr8=0.66,recall8=0.90,abstention-balanced=0.85;nonregression:recall20,mrr8,abstention-balanced", "compact/1", "corpus-text-counts/1", "filter-gate/1")
	if got := s.Recipe(stage.Snapshot{}); got != want {
		t.Fatalf("recipe = %s, want %s", got, want)
	}
}

func TestResolve_RejectsInvalidReport(t *testing.T) {
	for _, tc := range []struct {
		name, status, country, rel string
		eligible                   bool
		valid                      bool
	}{
		{"incomplete", "incomplete", "vn", "20261109", false, false},
		{"eligible", "complete", "vn", "20261109", true, false},
		{"country", "complete", "us", "20261109", false, false},
		{"release", "complete", "vn", "20261108", false, false},
		{"valid", "complete", "vn", "20261109", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			artifacts := artifact.Store{Root: root}
			hash := strings.Repeat("a", 64)
			art, err := artifacts.Publish(context.Background(), "eval", hash, hash, func(dir string) error {
				data, err := json.Marshal(evaluation.Report{Status: tc.status, Country: tc.country, Rel: tc.rel, ReleaseEligible: tc.eligible, EvaluationDate: retrieval.Date{Year: 2026, Month: 11, Day: 9}})
				if err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(dir, "report.json"), data, 0600)
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = New(Config{}).Resolve(context.Background(), stage.Env{Artifacts: artifacts}, stage.Run{Country: "vn", Rel: "20261109"}, stage.Item{Key: "vn/20261109", InputHash: hash}, art)
			if tc.valid {
				if err != nil {
					t.Fatalf("valid report: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "invalid evaluation report") {
				t.Fatalf("expected invalid evaluation report, got %v", err)
			}
		})
	}
}

func TestCheckInputHash_DistinguishesErrorsAndMismatch(t *testing.T) {
	s := New(Config{})
	err := s.checkInputHash(artifact.Result{}, stage.Run{Rel: "bad"}, "x")
	if err == nil || !strings.Contains(err.Error(), "invalid release date") || strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("input error: %v", err)
	}
	err = s.checkInputHash(artifact.Result{}, stage.Run{Rel: "20261109"}, "wrong")
	if err == nil || !strings.Contains(err.Error(), "evaluation input mismatch") {
		t.Fatalf("mismatch error: %v", err)
	}
}

func TestEvalStage_PlanDoesNotRestoreProcessDoes(t *testing.T) {
	sentinel := errors.New("restore called")
	calls := 0
	s := New(Config{})
	s.lookup = func(context.Context, stage.Env, stage.Run) (artifact.Result, error) {
		return artifact.Result{SHA256: "build"}, nil
	}
	s.ensure = func(context.Context, stage.Env, stage.Run) (build.Database, artifact.Result, error) {
		calls++
		return build.Database{}, artifact.Result{}, sentinel
	}
	run := stage.Run{Country: "vn", Rel: "20261109"}
	items, err := s.Plan(context.Background(), stage.Env{}, run)
	if err != nil || calls != 0 || len(items) != 1 {
		t.Fatalf("plan restored: items=%v calls=%d error=%v", items, calls, err)
	}
	_, err = s.Process(context.Background(), stage.Env{}, run, items[0])
	if !errors.Is(err, sentinel) || calls != 1 {
		t.Fatalf("process did not restore: calls=%d error=%v", calls, err)
	}
}

func TestProcess_VerifiesBuildOnce(t *testing.T) {
	s := New(Config{})
	calls, lookups := 0, 0
	s.lookup = func(context.Context, stage.Env, stage.Run) (artifact.Result, error) {
		lookups++
		return artifact.Result{SHA256: "build"}, nil
	}
	s.ensure = func(context.Context, stage.Env, stage.Run) (build.Database, artifact.Result, error) {
		calls++
		return build.Database{URL: "://invalid"}, artifact.Result{SHA256: "build"}, nil
	}
	run := stage.Run{Country: "vn", Rel: "20261109"}
	items, err := s.Plan(context.Background(), stage.Env{}, run)
	if err != nil {
		t.Fatal(err)
	}
	calls, lookups = 0, 0
	_, err = s.Process(context.Background(), stage.Env{}, run, items[0])
	if err == nil || calls != 1 || lookups != 0 {
		t.Fatalf("ensures=%d lookups=%d error=%v", calls, lookups, err)
	}
}

func TestEvalStage_DevelopmentFloorGaps(t *testing.T) {
	r := evaluation.Report{
		Floors: []evaluation.Floor{{Metric: evaluation.Metric{Mode: "exact", Name: "current_law_precision"}, Status: "fail"}},
		Cohorts: map[string]evaluation.Cohort{"development": {Floors: []evaluation.Floor{
			{Metric: evaluation.Metric{Mode: "exact", Name: "current_law_precision"}, Status: "fail"},
			{Metric: evaluation.Metric{Mode: "ann", Name: "current_law_precision"}, Status: "unavailable"},
			{Metric: evaluation.Metric{Mode: "exact", Name: "current_law_precision"}, Status: "pass"},
		}}},
	}
	want := []stage.Gap{
		{Kind: "eval_floor_failed", Detail: "exact:current_law_precision"},
		{Kind: "eval_floor_failed", Detail: "development:exact:current_law_precision"},
		{Kind: "eval_floor_failed", Detail: "development:ann:current_law_precision"},
	}
	got := reportGaps(r)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("gaps = %+v, want %+v", got, want)
	}
}

func TestEvalStage_NotRunDevelopmentNoGap(t *testing.T) {
	r := evaluation.Report{Cohorts: map[string]evaluation.Cohort{"development": {Floors: []evaluation.Floor{{Status: "not_run"}, {Status: "not_run"}}}}}
	if gaps := reportGaps(r); len(gaps) != 0 {
		t.Fatalf("not_run gaps: %+v", gaps)
	}
}

func TestEvalStage_DevelopmentHashInInput(t *testing.T) {
	s := New(Config{Suite: evaluation.Suite{Hashes: evaluation.FixtureHashes{Development: "first"}}})
	run := stage.Run{Rel: "20990101"}
	first, err := s.inputHash(artifact.Result{SHA256: "build"}, run)
	if err != nil {
		t.Fatal(err)
	}
	s.cfg.Suite.Hashes.Development = "second"
	second, err := s.inputHash(artifact.Result{SHA256: "build"}, run)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("development fixture omitted from evaluation input hash")
	}
}

func TestEvalStage_NoCapOverride(t *testing.T) {
	sentinel := errors.New("evaluation reached")
	s := New(Config{})
	s.lookup = func(context.Context, stage.Env, stage.Run) (artifact.Result, error) {
		return artifact.Result{SHA256: "build"}, nil
	}
	s.ensure = func(context.Context, stage.Env, stage.Run) (build.Database, artifact.Result, error) {
		return build.Database{}, artifact.Result{SHA256: "build"}, nil
	}
	s.evaluate = func(_ context.Context, _ stage.Env, _ stage.Run, _ build.Database, _ artifact.Result, opts evaluation.Options) (evaluation.Report, evaluation.Cache, error) {
		if opts.CapOverride != nil || opts.HistoricalPolicy != nil || opts.NonCurrentTail != nil || opts.NotYetEligible {
			t.Error("pipeline stage passed a research override")
		}
		return evaluation.Report{}, evaluation.Cache{}, sentinel
	}
	run := stage.Run{Country: "vn", Rel: "20990101"}
	items, err := s.Plan(context.Background(), stage.Env{}, run)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Process(context.Background(), stage.Env{}, run, items[0])
	if !errors.Is(err, sentinel) {
		t.Fatalf("evaluation not reached: %v", err)
	}
}

func TestEvaluate_ForwardsCapOverride(t *testing.T) {
	s := New(Config{})
	s.ensure = func(context.Context, stage.Env, stage.Run) (build.Database, artifact.Result, error) {
		return build.Database{}, artifact.Result{}, nil
	}
	s.evaluate = func(_ context.Context, _ stage.Env, _ stage.Run, _ build.Database, _ artifact.Result, opts evaluation.Options) (evaluation.Report, evaluation.Cache, error) {
		return evaluation.Report{CapOverride: opts.CapOverride}, evaluation.Cache{}, nil
	}
	cap, err := retrieval.ParseCapOverride("4")
	if err != nil {
		t.Fatal(err)
	}
	report, _, err := s.Evaluate(context.Background(), stage.Env{}, stage.Run{}, evaluation.Options{CapOverride: cap})
	if err != nil {
		t.Fatal(err)
	}
	if report.CapOverride == nil || *report.CapOverride != 4 {
		t.Fatalf("cap = %v, want 4", report.CapOverride)
	}
}

func TestEvaluate_ForwardsValidityOverrides(t *testing.T) {
	s := New(Config{})
	s.ensure = func(context.Context, stage.Env, stage.Run) (build.Database, artifact.Result, error) {
		return build.Database{}, artifact.Result{}, nil
	}
	s.evaluate = func(_ context.Context, _ stage.Env, _ stage.Run, _ build.Database, _ artifact.Result, opts evaluation.Options) (evaluation.Report, evaluation.Cache, error) {
		return evaluation.Report{ValidityOverrides: opts.ValidityOverrides}, evaluation.Cache{}, nil
	}
	history, tail := "agent", 3
	policy := evaluation.ValidityOverrides{HistoricalPolicy: &history, NonCurrentTail: &tail, NotYetEligible: true}
	report, _, err := s.Evaluate(context.Background(), stage.Env{}, stage.Run{}, evaluation.Options{ValidityOverrides: policy})
	if err != nil {
		t.Fatal(err)
	}
	if report.HistoricalPolicy == nil || *report.HistoricalPolicy != "agent" || report.NonCurrentTail == nil || *report.NonCurrentTail != 3 || !report.NotYetEligible {
		t.Fatalf("validity overrides: %+v", report.ValidityOverrides)
	}
}

func TestEvalStage_NoAbstentionGap(t *testing.T) {
	r := evaluation.Report{Targets: []evaluation.Target{{Metric: evaluation.Metric{Name: "abstention_balanced_accuracy"}, Status: "below"}}, Floors: []evaluation.Floor{{Metric: evaluation.Metric{Name: "current_law_precision", Mode: "exact"}, Status: "pass"}}}
	if gaps := reportGaps(r); len(gaps) != 0 {
		t.Fatalf("gaps = %+v", gaps)
	}
}

func TestRecipe_DoesNotReuseOldScorerArtifact(t *testing.T) {
	oldRecipe := stage.RecipeHash("eval", "agent-eval/2", "legacy-matcher/1", "product-matcher/1", "recall,mrr,current-law-precision,abstention,evidence,ann/1", "", "diacritics_required:Vui lòng nhập câu hỏi tiếng Việt có dấu hoặc số hiệu văn bản.", "cache/1:200:100", "nearest-rank", "gates:0.99;targets:recall20=0.90,mrr8=0.66,recall8=0.90,abstention-balanced=0.85;nonregression:recall20,mrr8,abstention-balanced", "compact/1", "corpus-text-counts/1")
	store := artifact.Store{Root: t.TempDir()}
	input := strings.Repeat("a", 64)
	_, err := store.Publish(context.Background(), "eval", input, oldRecipe, func(dir string) error {
		return os.WriteFile(filepath.Join(dir, "report.json"), []byte(`{"scorer":"agent-eval/2"}`), 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	_, found, err := store.Open("eval", input, New(Config{}).Recipe(stage.Snapshot{}))
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("reused artifact scored with agent-eval/2")
	}
}
