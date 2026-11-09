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
)

func TestEvalCLI_ExitCodes(t *testing.T) {
	for _, tc := range []struct {
		status  string
		require bool
		want    int
	}{{"pass", true, 0}, {"fail", true, 2}, {"unavailable", true, 2}, {"fail", false, 0}} {
		r := evaluation.Report{Status: "complete", QualityPass: true, Floors: make([]evaluation.Floor, 8)}
		for i := range r.Floors {
			r.Floors[i].Status = tc.status
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
	r := evaluation.Report{Status: "complete", Floors: make([]evaluation.Floor, 8)}
	for i := range r.Floors {
		r.Floors[i].Status = "pass"
	}
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
