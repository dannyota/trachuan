package eval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"danny.vn/trachuan/internal/retrieval"
)

func frozenReport(t *testing.T, artifacts artifact.Store, data string) stage.PreviousReport {
	t.Helper()
	hash := strings.Repeat("c", 64)
	result, err := artifacts.Publish(context.Background(), "previous-report", hash, hash, func(dir string) error {
		return os.WriteFile(filepath.Join(dir, stage.PreviousReportFile), []byte(data), 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(data))
	return stage.PreviousReport{SHA256: hex.EncodeToString(sum[:]), Path: result.Path}
}

func decisionStage(capture *evaluation.Options, sentinel error) *Stage {
	s := New(Config{Previous: "/nonexistent/cli-report.json"})
	s.lookup = func(context.Context, stage.Env, stage.Run) (artifact.Result, error) {
		return artifact.Result{SHA256: "build"}, nil
	}
	s.ensure = func(context.Context, stage.Env, stage.Run) (build.Database, artifact.Result, error) {
		return build.Database{}, artifact.Result{SHA256: "build"}, nil
	}
	s.evaluate = func(_ context.Context, _ stage.Env, _ stage.Run, _ build.Database, _ artifact.Result, opts evaluation.Options) (evaluation.Report, evaluation.Cache, error) {
		*capture = opts
		return evaluation.Report{}, evaluation.Cache{}, sentinel
	}
	return s
}

func decidedRun(decision stage.PreviousReport) stage.Run {
	return stage.Run{Country: "vn", Rel: "20990102_1", Snapshot: stage.Snapshot{PreviousReport: &decision}}
}

func TestInputHash_FrozenDecisionReplacesConfiguredReport(t *testing.T) {
	s := New(Config{Previous: "/nonexistent/cli-report.json"})
	art := artifact.Result{SHA256: "build"}
	first, err := s.inputHash(art, decidedRun(stage.PreviousReport{SHA256: strings.Repeat("a", 64)}))
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.inputHash(art, decidedRun(stage.PreviousReport{SHA256: strings.Repeat("b", 64)}))
	if err != nil {
		t.Fatal(err)
	}
	none, err := s.inputHash(art, decidedRun(stage.PreviousReport{SHA256: stage.NoPreviousReport}))
	if err != nil {
		t.Fatal(err)
	}
	s.cfg.Previous = ""
	legacy, err := s.inputHash(art, stage.Run{Country: "vn", Rel: "20990102_1"})
	if err != nil {
		t.Fatal(err)
	}
	if first == second || first == none || none == legacy {
		t.Fatalf("hashes collide: %s %s %s %s", first, second, none, legacy)
	}
	want := stage.RecipeHash("eval-input", "2", "build", "", "", "", "", "", strings.Repeat("a", 64), "2099-01-02", "exact,ann")
	if first != want {
		t.Fatalf("input hash = %s, want %s", first, want)
	}
}

func TestInputHash_RunWithoutDecisionStillReadsConfiguredReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	if err := os.WriteFile(path, []byte("prior"), 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("prior"))
	got, err := New(Config{Previous: path}).inputHash(artifact.Result{SHA256: "build"}, stage.Run{Rel: "20990101"})
	if err != nil {
		t.Fatal(err)
	}
	if want := stage.RecipeHash("eval-input", "2", "build", "", "", "", "", "", hex.EncodeToString(sum[:]), "2099-01-01", "exact,ann"); got != want {
		t.Fatalf("input hash = %s, want %s", got, want)
	}
}

func TestProcess_ComparesWithFrozenReportNotConfig(t *testing.T) {
	sentinel := errors.New("evaluation reached")
	artifacts := artifact.Store{Root: t.TempDir()}
	decision := frozenReport(t, artifacts, `{"rel":"20990101_1"}`)
	var opts evaluation.Options
	s := decisionStage(&opts, sentinel)
	env := stage.Env{Artifacts: artifacts}
	run := decidedRun(decision)
	items, err := s.Plan(context.Background(), env, run)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Process(context.Background(), env, run, items[0]); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
	if want := filepath.Join(artifacts.Abs(decision.Path), stage.PreviousReportFile); opts.PreviousReport != want {
		t.Fatalf("previous = %q, want %q", opts.PreviousReport, want)
	}
}

func TestProcess_NoneDecisionComparesWithNothing(t *testing.T) {
	sentinel := errors.New("evaluation reached")
	var opts evaluation.Options
	s := decisionStage(&opts, sentinel)
	run := decidedRun(stage.PreviousReport{SHA256: stage.NoPreviousReport})
	items, err := s.Plan(context.Background(), stage.Env{}, run)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Process(context.Background(), stage.Env{}, run, items[0]); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
	if opts.PreviousReport != "" {
		t.Fatalf("previous = %q", opts.PreviousReport)
	}
}

func TestProcess_RefusesChangedFrozenReport(t *testing.T) {
	artifacts := artifact.Store{Root: t.TempDir()}
	decision := frozenReport(t, artifacts, `{"rel":"20990101_1"}`)
	decision.SHA256 = strings.Repeat("d", 64)
	var opts evaluation.Options
	s := decisionStage(&opts, nil)
	env := stage.Env{Artifacts: artifacts}
	run := decidedRun(decision)
	items, err := s.Plan(context.Background(), env, run)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Process(context.Background(), env, run, items[0]); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("err = %v", err)
	}
}

func TestProcess_RunWithoutDecisionKeepsConfiguredReport(t *testing.T) {
	sentinel := errors.New("evaluation reached")
	path := filepath.Join(t.TempDir(), "report.json")
	if err := os.WriteFile(path, []byte("prior"), 0600); err != nil {
		t.Fatal(err)
	}
	var opts evaluation.Options
	s := decisionStage(&opts, sentinel)
	s.cfg.Previous = path
	run := stage.Run{Country: "vn", Rel: "20261118_1"}
	items, err := s.Plan(context.Background(), stage.Env{}, run)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Process(context.Background(), stage.Env{}, run, items[0]); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
	if opts.PreviousReport != path {
		t.Fatalf("previous = %q", opts.PreviousReport)
	}
}

func TestEvaluate_DiagnosticUsesConfiguredReportOnDecidedRun(t *testing.T) {
	var opts evaluation.Options
	s := decisionStage(&opts, nil)
	if _, _, err := s.Evaluate(context.Background(), stage.Env{}, decidedRun(stage.PreviousReport{SHA256: stage.NoPreviousReport}), evaluation.Options{}); err != nil {
		t.Fatal(err)
	}
	if opts.PreviousReport != s.cfg.Previous {
		t.Fatalf("previous = %q", opts.PreviousReport)
	}
}

func TestResolve_ItemDoneOnlyWhenReportMatchesDecision(t *testing.T) {
	frozen := strings.Repeat("a", 64)
	previous := func(sum string) any {
		return map[string]any{"previous": map[string]string{"country": "vn", "rel": "20990101_1", "report_sha256": sum}}
	}
	reason := map[string]any{"reason": "no previous report"}
	for _, tc := range []struct {
		name     string
		decision *stage.PreviousReport
		diff     any
		valid    bool
	}{
		{"matching report", &stage.PreviousReport{SHA256: frozen}, previous(frozen), true},
		{"other report", &stage.PreviousReport{SHA256: frozen}, previous(strings.Repeat("b", 64)), false},
		{"incompatible comparison", &stage.PreviousReport{SHA256: frozen}, map[string]any{"reason": "incompatible scorer version"}, false},
		{"none without previous", &stage.PreviousReport{SHA256: stage.NoPreviousReport}, reason, true},
		{"none with previous", &stage.PreviousReport{SHA256: stage.NoPreviousReport}, previous(frozen), false},
		{"older run ignores previous", nil, previous(frozen), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			artifacts := artifact.Store{Root: t.TempDir()}
			hash := strings.Repeat("e", 64)
			art, err := artifacts.Publish(context.Background(), "eval", hash, hash, func(dir string) error {
				data, marshalErr := json.Marshal(evaluation.Report{Status: "complete", Country: "vn", Rel: "20990102_1", PreviousDiff: tc.diff, EvaluationDate: retrieval.Date{Year: 2099, Month: 1, Day: 2}})
				if marshalErr != nil {
					return marshalErr
				}
				return os.WriteFile(filepath.Join(dir, "report.json"), data, 0600)
			})
			if err != nil {
				t.Fatal(err)
			}
			run := stage.Run{Country: "vn", Rel: "20990102_1", Snapshot: stage.Snapshot{PreviousReport: tc.decision}}
			_, err = New(Config{}).Resolve(context.Background(), stage.Env{Artifacts: artifacts}, run, stage.Item{Key: "vn/20990102_1", InputHash: hash}, art)
			if tc.valid && err != nil {
				t.Fatalf("valid report: %v", err)
			}
			if !tc.valid && (err == nil || !strings.Contains(err.Error(), "previous report decision")) {
				t.Fatalf("err = %v", err)
			}
			var syntax *json.SyntaxError
			if errors.As(err, &syntax) {
				t.Fatal("decision failure would become a source data gap")
			}
		})
	}
}
