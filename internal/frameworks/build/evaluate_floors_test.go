package build

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func floorEntry(value int, change string) string {
	return fmt.Sprintf("- release: '20990101'\n  report_sha256: %s\n  reviewer: coordinator\n  date: '2099-01-01'\n  cohort_change: %q\n  floors:\n    open: {recall_at_8: %d, mrr_at_8: %d}\n    filtered: {recall_at_8: %d, mrr_at_8: %d}\n    abstention_balanced_accuracy: %d\n", strings.Repeat("a", 64), change, value, value, value, value, value)
}

func TestFloors_ClosedShapeAndLowering(t *testing.T) {
	raw := "baselines:\n" + floorEntry(50, "")
	floors, err := parseFloors([]byte(raw))
	require.NoError(t, err)
	require.Len(t, floors, 1)
	for _, bad := range []string{
		raw + "unknown: true\n", strings.Replace(raw, "mrr_at_8: 50", "mrr_at_8: 50.5", 1), strings.Replace(raw, "recall_at_8: 50", "recall_at_8: 101", 1), strings.Replace(raw, "cohort_change: \"\"\n", "", 1), raw + "---\nbaselines: []\n", "baselines:\n" + floorEntry(50, "") + floorEntry(49, ""), strings.Replace(raw, "open: {recall_at_8: 50, mrr_at_8: 50}", "open: {recall_at_8: 50}", 1),
	} {
		_, parseErr := parseFloors([]byte(bad))
		require.Error(t, parseErr)
	}
	floors, err = parseFloors([]byte("baselines:\n" + floorEntry(50, "") + floorEntry(49, "new cases")))
	require.NoError(t, err)
	require.Equal(t, 49, *floors[1].Floors.Open.Recall)
}

func TestFloors_GatesAndUnavailable(t *testing.T) {
	baselines, err := parseFloors([]byte("baselines:\n" + floorEntry(50, "")))
	require.NoError(t, err)
	cases := []CaseResult{{Adjudication: "accepted", Access: "free", Results: map[string]map[string]CaseOutcome{"free-only": {"open": {Rank: 2}, "filtered": {Rank: 1}}}}, {Adjudication: "accepted", Negative: true, Results: map[string]map[string]CaseOutcome{"free-only": {"open": {Abstention: "abstained"}}}}}
	require.True(t, checkFloors(baselines, "hash", ComputeMetrics(cases, "public", nil)).Pass)
	cases[0].Results["free-only"]["open"] = CaseOutcome{Error: true}
	require.False(t, checkFloors(baselines, "hash", ComputeMetrics(cases, "public", nil)).Pass)
	require.False(t, checkFloors(nil, "", CohortMetrics{}).Pass)
}
