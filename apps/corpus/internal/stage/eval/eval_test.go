package eval

import (
	"context"
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

func TestEvalStage_ReportStatusAndGaps(t *testing.T) {
	r := evaluation.Report{Status: "complete", Floors: []evaluation.Floor{{Metric: evaluation.Metric{Mode: retrieval.Mode("exact"), Name: "recall"}, Status: "fail"}, {Metric: evaluation.Metric{Mode: retrieval.Mode("ann"), Name: "mrr"}, Status: "unavailable"}, {Status: "pass"}}, Rejections: []evaluation.Rejection{{CaseID: "bad", Passed: false}, {CaseID: "ok", Passed: true}}}
	gaps := reportGaps(r)
	if len(gaps) != 3 || gaps[0].Kind != "eval_floor_failed" || gaps[0].Detail != "exact:recall" || gaps[1].Detail != "ann:mrr" || gaps[2].Kind != "eval_rejection_failed" || gaps[2].Detail != "bad" {
		t.Fatalf("gaps: %+v", gaps)
	}
}

func TestResolve_RejectsInvalidReport(t *testing.T) {
	for _, tc := range []struct {
		name, status, country, rel string
		eligible                   bool
	}{
		{"incomplete", "incomplete", "vn", "20261109", false},
		{"eligible", "complete", "vn", "20261109", true},
		{"country", "complete", "us", "20261109", false},
		{"release", "complete", "vn", "20261108", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			artifacts := artifact.Store{Root: root}
			hash := strings.Repeat("a", 64)
			art, err := artifacts.Publish(context.Background(), "eval", hash, hash, func(dir string) error {
				data, err := json.Marshal(evaluation.Report{Status: tc.status, Country: tc.country, Rel: tc.rel, ReleaseEligible: tc.eligible})
				if err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(dir, "report.json"), data, 0600)
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = New(Config{}).Resolve(context.Background(), stage.Env{Artifacts: artifacts}, stage.Run{Country: "vn", Rel: "20261109"}, stage.Item{Key: "vn/20261109", InputHash: hash}, art)
			if err == nil {
				t.Fatal("accepted invalid report")
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
	s.ensure = func(context.Context, stage.Env, stage.Run) (build.Database, error) {
		calls++
		return build.Database{}, sentinel
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
