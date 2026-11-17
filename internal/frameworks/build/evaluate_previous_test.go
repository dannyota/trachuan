package build

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSnapshot_PreviousReportInputIsFrozen(t *testing.T) {
	work := fixture(t)
	previous := []byte(`{"version":1,"rel":"20990101","quality_pass":true,"cases":[]}`)
	path := filepath.Join(os.Getenv("FRAMEWORKS_SOURCE_DIR"), "trachuan/previous-report.json")
	require.NoError(t, os.WriteFile(path, previous, 0600))
	require.NoError(t, testRun(context.Background(), "20990102", &bytes.Buffer{}))
	frozen := filepath.Join(work, "20990102/snapshot/trachuan/previous-report.json")
	require.FileExists(t, frozen)
	reports := readRows[EvaluationReport](t, filepath.Join(work, "20990102/evaluate/report.json"))
	require.NotNil(t, reports[0].Previous)
	require.Equal(t, digest(previous), reports[0].Previous.ReportSHA256)
	require.Equal(t, "20990101", reports[0].Previous.Rel)
	require.Equal(t, "evaluated", reports[0].PreviousDiff.Status)
	require.NoError(t, os.WriteFile(path, []byte("changed after snapshot"), 0600))
	require.NoError(t, testRun(context.Background(), "20990102", &bytes.Buffer{}))
	actual, err := os.ReadFile(frozen)
	require.NoError(t, err)
	require.Equal(t, previous, actual)
}

func passingQualityReport() EvaluationReport {
	threshold := .5
	return EvaluationReport{PreviousDiff: previousComparison{Status: "not_applicable", Pass: true}, CalibrationSHA256: "hash", Threshold: &threshold, EvaluationResult: EvaluationResult{Restored: true, RetrievalRecipeStatus: "measured", Invariant: InvariantResult{Pass: true}, Cohorts: map[string]CohortMetrics{"public": {Filter: FilterGate{Pass: true}}, "licensed": {Filter: FilterGate{Pass: true}}}}, Floors: map[string]FloorResult{"public": {Pass: true}, "licensed": {Pass: true}}}
}
func TestQualityPass_PreviousRegressionFails(t *testing.T) {
	report := passingQualityReport()
	require.NoError(t, json.Unmarshal([]byte(`{"previous":{"rel":"20990101","report_sha256":"hash"},"previous_diff":{"status":"evaluated","pass":false}}`), &report))
	require.False(t, qualityPass(report))
}

func pairedCaseFixture(id string, open, filtered int) CaseResult {
	r := CaseResult{ID: id, Adjudication: "accepted", Code: "fw-alpha", Version: "1", Access: "free", Target: "fw:fw-alpha:1#A", Cohorts: map[string]map[string]CaseOutcome{}}
	for _, cohort := range []string{"public", "licensed"} {
		r.Cohorts[cohort] = map[string]CaseOutcome{"open": {Rank: open, Miss: open == 0}, "filtered": {Rank: filtered, Miss: filtered == 0}}
	}
	return r
}
func TestPreviousComparison_PairedMetricsAndRegressions(t *testing.T) {
	for _, test := range []struct {
		name          string
		before, after []CaseResult
		pass          bool
		lost          []string
	}{
		{name: "unchanged", before: []CaseResult{pairedCaseFixture("a", 2, 2)}, after: []CaseResult{pairedCaseFixture("a", 2, 2)}, pass: true},
		{name: "gain", before: []CaseResult{pairedCaseFixture("a", 0, 0)}, after: []CaseResult{pairedCaseFixture("a", 1, 1)}, pass: true},
		{name: "mrr falls", before: []CaseResult{pairedCaseFixture("a", 1, 1)}, after: []CaseResult{pairedCaseFixture("a", 2, 2)}},
		{name: "lost hit despite aggregate gain", before: []CaseResult{pairedCaseFixture("a", 8, 8), pairedCaseFixture("b", 0, 0)}, after: []CaseResult{pairedCaseFixture("a", 0, 0), pairedCaseFixture("b", 1, 1)}, lost: []string{"a"}},
		{name: "equal rational mrr", before: []CaseResult{pairedCaseFixture("a", 3, 3), pairedCaseFixture("b", 6, 6), pairedCaseFixture("c", 2, 2)}, after: []CaseResult{pairedCaseFixture("a", 2, 2), pairedCaseFixture("b", 3, 3), pairedCaseFixture("c", 6, 6)}, pass: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ref := &previousReference{Rel: "20990101", ReportSHA256: "hash"}
			diff := comparePreviousCases(ref, test.before, test.after)
			require.Equal(t, test.pass, diff.Pass)
			require.Equal(t, "evaluated", diff.Status)
			m := diff.Cohorts["public"].Modes["open"]
			require.Equal(t, len(test.before), m.PreviousRecall.Denominator)
			require.Equal(t, len(test.before), m.CurrentRecall.Denominator)
			require.ElementsMatch(t, test.lost, m.LostHits)
			report := passingQualityReport()
			report.Previous = ref
			report.PreviousDiff = diff
			require.Equal(t, test.pass, qualityPass(report))
		})
	}
}
func TestPreviousComparison_ExcludesCohortChangesAndDiagnostics(t *testing.T) {
	before := []CaseResult{pairedCaseFixture("stable", 2, 2), pairedCaseFixture("rejected", 1, 1), pairedCaseFixture("retargeted", 1, 1), pairedCaseFixture("unverified", 1, 1), pairedCaseFixture("outside", 1, 1), pairedCaseFixture("removed", 1, 1)}
	before[3].Adjudication = "unverified"
	before[4].Adjudication = "out_of_release"
	before[4].Target = ""
	after := []CaseResult{pairedCaseFixture("stable", 2, 2), pairedCaseFixture("rejected", 0, 0), pairedCaseFixture("retargeted", 0, 0), pairedCaseFixture("unverified", 0, 0), pairedCaseFixture("outside", 1, 1), pairedCaseFixture("added", 0, 0)}
	after[1].Adjudication = "rejected"
	after[2].Target = "fw:fw-alpha:1#B"
	after[3].Adjudication = "unverified"
	result := comparePreviousCases(&previousReference{}, before, after)
	require.True(t, result.Pass)
	require.Equal(t, []string{"stable"}, result.Cohorts["public"].Modes["open"].Cases)
	require.Equal(t, []string{"rejected"}, result.Changes.Rejected)
	require.Equal(t, []string{"retargeted"}, result.Changes.Retargeted)
	require.Equal(t, []string{"outside"}, result.Changes.NewlyInRelease)
	require.Empty(t, result.Changes.NewVersions)
	require.Equal(t, []string{"added"}, result.Changes.Added)
	require.Equal(t, []string{"removed"}, result.Changes.Removed)
}
func TestPreviousComparison_CohortAndModeGates(t *testing.T) {
	previous := []CaseResult{pairedCaseFixture("free", 1, 1), pairedCaseFixture("licensed", 1, 1)}
	current := []CaseResult{pairedCaseFixture("free", 1, 1), pairedCaseFixture("licensed", 0, 0)}
	previous[1].Access, current[1].Access = "licensed", "licensed"
	result := comparePreviousCases(&previousReference{}, previous, current)
	require.False(t, result.Pass)
	require.True(t, result.Cohorts["public"].Modes["open"].Pass)
	require.False(t, result.Cohorts["licensed"].Modes["open"].Pass)
	result = comparePreviousCases(&previousReference{}, []CaseResult{pairedCaseFixture("p", 1, 1)}, []CaseResult{pairedCaseFixture("p", 1, 0)})
	require.True(t, result.Cohorts["public"].Modes["open"].Pass)
	require.False(t, result.Cohorts["public"].Modes["filtered"].Pass)
}
func TestPreviousComparison_UnavailableAndAbsent(t *testing.T) {
	absent := comparePreviousCases(nil, nil, nil)
	require.True(t, absent.Pass)
	require.Equal(t, "not_applicable", absent.Status)
	empty := comparePreviousCases(&previousReference{}, nil, nil)
	require.False(t, empty.Pass)
	require.Equal(t, "unavailable", empty.Cohorts["public"].Modes["open"].CurrentRecall.Status)
	for _, change := range []func(*CaseResult){
		func(c *CaseResult) { c.Cohorts["public"]["open"] = CaseOutcome{Error: true} },
		func(c *CaseResult) { delete(c.Cohorts["public"], "open") },
		func(c *CaseResult) { c.Cohorts["public"]["open"] = CaseOutcome{Rank: 9} },
	} {
		c := pairedCaseFixture("p", 1, 1)
		change(&c)
		result := comparePreviousCases(&previousReference{}, []CaseResult{pairedCaseFixture("p", 1, 1)}, []CaseResult{c})
		require.False(t, result.Pass)
		require.Equal(t, "unavailable", result.Cohorts["public"].Modes["open"].CurrentRecall.Status)
	}
}
func TestPreviousReport_RejectsInvalidInputWithoutText(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "snapshot/trachuan")
	require.NoError(t, os.MkdirAll(dir, 0700))
	_, _, err := readPreviousReport(base, "20990102")
	require.NoError(t, err)
	for _, raw := range []string{
		`{"version":1,"rel":"20990101","quality_pass":false,"cases":[]}`,
		`{"version":1,"rel":"20990102","quality_pass":true,"cases":[]}`,
		`{"version":1,"rel":"20990101","quality_pass":true,"cases":[{"id":"a","adjudication":"unverified"},{"id":"a","adjudication":"unverified"}]}`,
		`{"version":1,"rel":"20990101","quality_pass":true,"cases":"invented private question"}`,
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "previous-report.json"), []byte(raw), 0600))
		_, _, err := readPreviousReport(base, "20990102")
		require.Error(t, err)
		require.NotContains(t, err.Error(), "invented private question")
	}
}

func previousCasesFixture(cases []CaseResult) previousReport {
	golden := make([]GoldenCase, 0, len(cases))
	for _, c := range cases {
		golden = append(golden, GoldenCase{Code: c.Code, Version: c.Version, Adjudication: c.Adjudication})
	}
	return previousReport{Cases: cases, CaseCounts: CountGoldenCases(golden)}
}
func comparePreviousCases(reference *previousReference, before, after []CaseResult) previousComparison {
	return comparePrevious(reference, previousCasesFixture(before), after, previousCasesFixture(after).CaseCounts)
}
