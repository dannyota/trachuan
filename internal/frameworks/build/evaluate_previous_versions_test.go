package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func versionCaseFixture(id, adjudication string) CaseResult {
	c := pairedCaseFixture(id, 1, 1)
	c.Version = "2"
	c.Adjudication = adjudication
	c.Target = "fw:fw-alpha:2#A"
	if adjudication == "out_of_release" {
		c.Target = ""
	}
	return c
}
func TestPreviousComparison_VersionMembership(t *testing.T) {
	for _, test := range []struct {
		name                      string
		before, after             []CaseResult
		beforeCounts, afterCounts []GoldenCaseCount
		want                      []string
	}{
		{name: "all new ids", before: []CaseResult{pairedCaseFixture("old", 1, 1)}, after: []CaseResult{versionCaseFixture("new", "accepted")}, beforeCounts: []GoldenCaseCount{{Code: "fw-alpha", Version: "1", Accepted: 1}}, afterCounts: []GoldenCaseCount{{Code: "fw-alpha", Version: "2", Accepted: 1}}, want: []string{"fw:fw-alpha:2"}},
		{name: "outside to in release with new ids", before: []CaseResult{versionCaseFixture("old", "out_of_release")}, after: []CaseResult{versionCaseFixture("new", "unverified")}, beforeCounts: []GoldenCaseCount{{Code: "fw-alpha", Version: "2", OutOfRelease: 1}}, afterCounts: []GoldenCaseCount{{Code: "fw-alpha", Version: "2", Unverified: 1}}, want: []string{"fw:fw-alpha:2"}},
		{name: "present in both despite case transition", before: []CaseResult{versionCaseFixture("entering", "out_of_release"), versionCaseFixture("previously-present", "rejected")}, after: []CaseResult{versionCaseFixture("entering", "accepted")}, beforeCounts: []GoldenCaseCount{{Code: "fw-alpha", Version: "2", Rejected: 1, OutOfRelease: 1}}, afterCounts: []GoldenCaseCount{{Code: "fw-alpha", Version: "2", Accepted: 1}}, want: []string{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := comparePrevious(&previousReference{}, previousReport{Cases: test.before, CaseCounts: test.beforeCounts}, test.after, test.afterCounts)
			require.Equal(t, test.want, result.Changes.NewVersions)
		})
	}
}

func TestPreviousReport_PreservesVersionCounts(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "snapshot/trachuan")
	require.NoError(t, os.MkdirAll(dir, 0700))
	report := EvaluationReport{Version: 1, Release: "20990101", QualityPass: true, CaseCounts: []GoldenCaseCount{{Code: "fw-alpha", Version: "1", Accepted: 1}, {Code: "fw-alpha", Version: "2", OutOfRelease: 1}}, EvaluationResult: EvaluationResult{Cases: []CaseResult{pairedCaseFixture("old", 1, 1), versionCaseFixture("outside", "out_of_release")}}}
	raw, err := json.Marshal(report)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "previous-report.json"), raw, 0600))
	reference, previous, err := readPreviousReport(base, "20990102")
	require.NoError(t, err)
	require.Equal(t, report.CaseCounts, previous.CaseCounts)
	result := comparePrevious(reference, previous, []CaseResult{versionCaseFixture("all-new-id", "rejected")}, []GoldenCaseCount{{Code: "fw-alpha", Version: "2", Rejected: 1}})
	require.Equal(t, []string{"fw:fw-alpha:2"}, result.Changes.NewVersions)
}
func TestPreviousComparison_VersionCountsAreAuthoritative(t *testing.T) {
	before := previousReport{Cases: []CaseResult{}, CaseCounts: []GoldenCaseCount{
		{Code: "fw-alpha", Version: "1", Accepted: 1},
		{Code: "fw-alpha", Version: "2", Rejected: 1},
		{Code: "fw-alpha", Version: "3", Unverified: 1},
		{Code: "fw-alpha", Version: "4", OutOfRelease: 1},
	}}
	after := []GoldenCaseCount{
		{Code: "fw-alpha", Version: "6", OutOfRelease: 7},
		{Code: "fw-alpha", Version: "5", Unverified: 1},
		{Code: "fw-alpha", Version: "4", Rejected: 1},
		{Code: "fw-alpha", Version: "3", Accepted: 1},
		{Code: "fw-alpha", Version: "2", Accepted: 1},
		{Code: "fw-alpha", Version: "1", Accepted: 1},
		{Accepted: 15},
	}
	result := comparePrevious(&previousReference{}, before, nil, after)
	require.Equal(t, []string{"fw:fw-alpha:4", "fw:fw-alpha:5"}, result.Changes.NewVersions)
}
