package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/internal/retrieval"
)

func TestEvalCLI_ExitCodes(t *testing.T) {
	for _, tc := range []struct {
		status  string
		require bool
		want    int
	}{{"pass", true, 0}, {"fail", true, 2}, {"unavailable", true, 2}, {"fail", false, 0}} {
		r := evaluation.Report{Status: "complete", QualityPass: true, Floors: make([]evaluation.Floor, 4)}
		for i := range r.Floors {
			r.Floors[i].Status = tc.status
			r.Floors[i].Mode = []retrieval.Mode{"exact", "exact", "ann", "ann"}[i]
			r.Floors[i].Name = []string{"current_law_precision", "abstention", "current_law_precision", "abstention"}[i]
			r.Floors[i].Arms = "hybrid"
			r.Floors[i].Limit = 8
		}
		if got := evalExit(r, tc.require); got != tc.want {
			t.Fatalf("%s %v: %d", tc.status, tc.require, got)
		}
	}
	if got := evalExit(evaluation.Report{Status: "incomplete"}, true); got != 1 {
		t.Fatalf("incomplete: %d", got)
	}
}

func TestEvalCLI_RequireFloorsRejectsFailedQuality(t *testing.T) {
	r := evaluation.Report{Status: "complete", Floors: make([]evaluation.Floor, 4)}
	for i := range r.Floors {
		r.Floors[i].Status = "pass"
		r.Floors[i].Mode = []retrieval.Mode{"exact", "exact", "ann", "ann"}[i]
		r.Floors[i].Name = []string{"current_law_precision", "abstention", "current_law_precision", "abstention"}[i]
		r.Floors[i].Arms = "hybrid"
		r.Floors[i].Limit = 8
	}
	r.QualityPass = true
	if got := evalExit(r, true); got != 0 {
		t.Fatalf("passing quality: got %d, want 0", got)
	}
	r.QualityPass = false
	if got := evalExit(r, true); got != 2 {
		t.Fatalf("got %d, want 2", got)
	}
}

type failingEvalCache struct{}

func (failingEvalCache) WriteFiles(string) error { return errors.New("cache write failed") }

func TestEvalOutput_AtomicAndRefusesNonempty(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "output")
	if err := writeEvaluationOutput(out, evaluation.Report{}, failingEvalCache{}); err == nil {
		t.Fatal("cache failure accepted")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("partial output exists: %v", err)
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "existing"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeEvaluationOutput(out, evaluation.Report{}, evaluation.Cache{}); err == nil {
		t.Fatal("nonempty output accepted")
	}
	if _, err := os.Stat(filepath.Join(out, "report.json")); !os.IsNotExist(err) {
		t.Fatalf("unexpected report: %v", err)
	}
}

func TestEvalOutput_WritesManifest(t *testing.T) {
	out := filepath.Join(t.TempDir(), "output")
	if err := writeEvaluationOutput(out, evaluation.Report{}, evaluation.Cache{}); err != nil {
		t.Fatal(err)
	}
	if _, err := artifact.Verify(out); err != nil {
		t.Fatalf("verify output: %v", err)
	}
}

func TestEvalCLI_OpenDatabaseErrorIncludesRelease(t *testing.T) {
	_, err := openEvalDB(context.Background(), "vn", "20261109", "://invalid")
	if err == nil || !strings.Contains(err.Error(), "corpus eval vn/20261109") {
		t.Fatalf("error = %v", err)
	}
}

func TestEvalOutput_ExistingEmptyDirectory(t *testing.T) {
	out := filepath.Join(t.TempDir(), "output")
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeEvaluationOutput(out, evaluation.Report{}, evaluation.Cache{}); err != nil {
		t.Fatal(err)
	}
	if _, err := artifact.Verify(out); err != nil {
		t.Fatal(err)
	}
}

func TestEvalCLI_RejectsNonemptyOutputBeforeEvaluation(t *testing.T) {
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "existing"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CORPUS_DATABASE_URL", "")
	var stderr bytes.Buffer
	if code := runEval(context.Background(), []string{"vn", "20261109", "--out", out}, &stderr, &stderr); code != 1 || !strings.Contains(stderr.String(), "output directory is not empty") {
		t.Fatalf("code=%d error=%s", code, stderr.String())
	}
}

func TestEvalCLI_CapFlag(t *testing.T) {
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "existing"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, cap := range []string{"1", "4", "50", "none"} {
		var stderr bytes.Buffer
		code := runEval(context.Background(), []string{"vn", "20261109", "--out", out, "--cap", cap}, &stderr, &stderr)
		if code != 1 || !strings.Contains(stderr.String(), "output directory is not empty") {
			t.Errorf("cap %s: code=%d error=%s", cap, code, stderr.String())
		}
	}
	for _, cap := range []string{"0", "51", "-1", "NONE", "1.5", "04"} {
		var stderr bytes.Buffer
		code := runEval(context.Background(), []string{"vn", "20261109", "--out", out, "--cap", cap}, &stderr, &stderr)
		if code != 1 || !strings.Contains(stderr.String(), "invalid cap") {
			t.Errorf("cap %s: code=%d error=%s", cap, code, stderr.String())
		}
	}
}

type replaceOutputWithFile struct{ out string }

func (w replaceOutputWithFile) WriteFiles(string) error {
	if err := os.Remove(w.out); err != nil {
		return err
	}
	return os.WriteFile(w.out, []byte("keep"), 0600)
}

func TestEvalOutput_PreservesFileCreatedDuringEvaluation(t *testing.T) {
	out := filepath.Join(t.TempDir(), "output")
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeEvaluationOutput(out, replaceOutputWithFile{out: out}, evaluation.Cache{}); err == nil {
		t.Fatal("accepted file as output directory")
	}
	data, err := os.ReadFile(out)
	if err != nil || string(data) != "keep" {
		t.Fatalf("file = %q, error = %v", data, err)
	}
}

func TestEvalCLI_RejectsDuplicateGates(t *testing.T) {
	r := evaluation.Report{Status: "complete", QualityPass: true, Floors: make([]evaluation.Floor, 4)}
	for i := range r.Floors {
		r.Floors[i] = evaluation.Floor{Metric: evaluation.Metric{Mode: "exact", Arms: "hybrid", Limit: 8, Name: "abstention"}, Status: "pass"}
	}
	if got := evalExit(r, true); got != 2 {
		t.Fatalf("duplicate gates exit %d", got)
	}
}

func TestEvalCLI_RequireFloorsDevelopment(t *testing.T) {
	floors := []evaluation.Floor{
		{Metric: evaluation.Metric{Mode: "exact", Arms: "hybrid", Limit: 8, Name: "current_law_precision"}, Status: "pass"},
		{Metric: evaluation.Metric{Mode: "exact", Arms: "hybrid", Limit: 8, Name: "abstention"}, Status: "pass"},
		{Metric: evaluation.Metric{Mode: "ann", Arms: "hybrid", Limit: 8, Name: "current_law_precision"}, Status: "pass"},
		{Metric: evaluation.Metric{Mode: "ann", Arms: "hybrid", Limit: 8, Name: "abstention"}, Status: "pass"},
	}
	for _, tc := range []struct {
		name, status string
		count, want  int
	}{
		{"pass", "pass", 2, 0}, {"fail", "fail", 2, 2}, {"unavailable", "unavailable", 2, 2},
		{"empty", "not_run", 0, 0}, {"populated not_run", "not_run", 2, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			development := append([]evaluation.Floor(nil), floors...)
			for i := range development {
				development[i].Status = tc.status
			}
			r := evaluation.Report{Status: "complete", QualityPass: true, Floors: floors, Cohorts: map[string]evaluation.Cohort{"development": {CaseCount: tc.count, Floors: development}}}
			if got := evalExit(r, true); got != tc.want {
				t.Fatalf("exit = %d, want %d", got, tc.want)
			}
			if got := evalExit(r, false); got != 0 {
				t.Fatalf("diagnostic exit = %d", got)
			}
		})
	}
	r := evaluation.Report{Status: "complete", QualityPass: true, Floors: floors, Rejections: []evaluation.Rejection{{Passed: false}}}
	if got := evalExit(r, true); got != 2 {
		t.Fatalf("failed rejection exit = %d", got)
	}
}
