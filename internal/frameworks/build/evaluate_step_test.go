package build

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRun_EvaluatesAndSnapshotsPublicFloors(t *testing.T) {
	work := fixture(t)
	require.NoError(t, os.WriteFile("sources/frameworks-floors.yaml", []byte("baselines: []\n"), 0600))
	var out bytes.Buffer
	require.NoError(t, testRun(context.Background(), "20990101", &out))
	require.FileExists(t, filepath.Join(work, "20990101/evaluate/report.json"))
	require.FileExists(t, filepath.Join(work, "20990101/snapshot/sources/frameworks-floors.yaml"))
	require.Contains(t, out.String(), "evaluate complete")
}

func TestEvaluationReport_QualityConditions(t *testing.T) {
	threshold := .5
	good := EvaluationReport{PreviousDiff: previousComparison{Status: "not_applicable", Pass: true}, CalibrationSHA256: "hash", Threshold: &threshold, EvaluationResult: EvaluationResult{Restored: true, RetrievalRecipeStatus: "measured", Invariant: InvariantResult{Pass: true}, Cohorts: map[string]CohortMetrics{"public": {Filter: FilterGate{Pass: true}}, "licensed": {Filter: FilterGate{Pass: true}}}}, Floors: map[string]FloorResult{"public": {Pass: true}, "licensed": {Pass: true}}}
	require.True(t, qualityPass(good))
	for _, change := range []func(*EvaluationReport){
		func(r *EvaluationReport) { r.Restored = false }, func(r *EvaluationReport) { r.RetrievalRecipeStatus = "placeholder" }, func(r *EvaluationReport) { r.Invariant.Pass = false }, func(r *EvaluationReport) { r.CalibrationSHA256 = "" }, func(r *EvaluationReport) { r.Threshold = nil }, func(r *EvaluationReport) { r.Gaps = []GapTotal{{Blocks: true, Count: 1}} }, func(r *EvaluationReport) { r.Floors = map[string]FloorResult{} }, func(r *EvaluationReport) { r.Cohorts = map[string]CohortMetrics{} },
	} {
		r := good
		change(&r)
		require.False(t, qualityPass(r))
	}
}

func TestEvaluationReport_ProvenanceAndMissingBaselines(t *testing.T) {
	work := fixture(t)
	require.NoError(t, testRun(context.Background(), "20990101", &bytes.Buffer{}))
	base := filepath.Join(work, "20990101")
	reports := readRows[EvaluationReport](t, filepath.Join(base, "evaluate/report.json"))
	require.Len(t, reports, 1)
	r := reports[0]
	require.False(t, r.QualityPass)
	require.Equal(t, "no_baseline", r.Floors["public"].Status)
	require.Equal(t, "no_baseline", r.Floors["licensed"].Status)
	require.Equal(t, "not_applicable", r.PreviousDiff.Status)
	require.Nil(t, r.Previous)
	require.Contains(t, r.PreviousDiff.Reason, "input")
	raw, err := os.ReadFile(filepath.Join(base, "build/build.json"))
	require.NoError(t, err)
	require.Equal(t, digest(raw), r.BuildManifestSHA256)
	require.Len(t, r.RecipeHashes, 7)
}

func TestRecipeSHA256_JSONBRoundTrip(t *testing.T) {
	r := stepRecipe("snapshot")
	raw, err := json.Marshal(r)
	require.NoError(t, err)
	var decoded any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, RecipeSHA256(r), RecipeSHA256(decoded))
}

type failingEvaluator struct{ fakeDatabase }

func (failingEvaluator) Evaluate(context.Context, EvaluationRequest, func(EvaluationResult) error) error {
	return errors.New("invented question text in engine failure")
}
func TestRun_EvaluationFailureIsPrivateAndRetryable(t *testing.T) {
	work := fixture(t)
	var out bytes.Buffer
	err := Run(context.Background(), "20990101", &out, fakeEmbedder{}, failingEvaluator{})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "invented question text")
	require.NoDirExists(t, filepath.Join(work, "20990101/evaluate"))
	out.Reset()
	require.NoError(t, testRun(context.Background(), "20990101", &out))
	require.Equal(t, 6, strings.Count(out.String(), "skipped"))
	require.Contains(t, out.String(), "evaluate complete")
}

func TestEvaluationStep_RejectsChangedBuiltCases(t *testing.T) {
	work := fixture(t)
	require.NoError(t, testRun(context.Background(), "20990101", &bytes.Buffer{}))
	base := filepath.Join(work, "20990101")
	require.NoError(t, os.WriteFile(filepath.Join(base, "build/cases.jsonl"), []byte("{\"id\":\"invented\"}\n"), 0600))
	err := evaluateStep(context.Background(), base, t.TempDir(), DatabaseRequest{Rel: "20990101"}, fakeDatabase{}, func() error { return nil })
	require.ErrorContains(t, err, "built case mapping mismatch")
}
