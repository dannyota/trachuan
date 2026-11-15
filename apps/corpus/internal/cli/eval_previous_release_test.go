package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/apps/corpus/internal/stage"
)

type previousReleaseStage struct{ options *evaluation.Options }

func (s previousReleaseStage) Evaluate(_ context.Context, _ stage.Env, _ stage.Run, opts evaluation.Options) (evaluation.Report, evaluation.Cache, error) {
	*s.options = opts
	return evaluation.Report{Status: "complete"}, evaluation.Cache{}, nil
}

func TestEvalCLI_PreviousReleaseMetadataForwarded(t *testing.T) {
	var got evaluation.Options
	prepare := func(_ context.Context, _, _, previous string) (evalTarget, error) {
		if previous != "previous-report.json" {
			t.Fatalf("previous: %s", previous)
		}
		return evalTarget{runner: previousReleaseStage{&got}, close: func() {}}, nil
	}
	var stderr bytes.Buffer
	code := runEvalWith(context.Background(), []string{"vn", "20990102_1", "--out", filepath.Join(t.TempDir(), "out"), "--previous", "previous-report.json", "--previous-release", "previous-release.json"}, &stderr, prepare)
	if code != 0 || got.PreviousRelease != "previous-release.json" {
		t.Fatalf("exit %d, metadata %q, error %s", code, got.PreviousRelease, stderr.String())
	}
}

func TestEvalCLI_PreviousReleaseMetadataRequiresReport(t *testing.T) {
	prepare := func(context.Context, string, string, string) (evalTarget, error) {
		t.Fatal("prepared without previous report")
		return evalTarget{}, nil
	}
	var stderr bytes.Buffer
	code := runEvalWith(context.Background(), []string{"vn", "20990102_1", "--out", filepath.Join(t.TempDir(), "out"), "--previous-release", "previous-release.json"}, &stderr, prepare)
	if code != 1 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
}
