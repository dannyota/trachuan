package build

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEvaluationMetrics_ArithmeticAndExclusion(t *testing.T) {
	cases := []CaseResult{
		{ID: "a", Adjudication: "accepted", Code: "fw-alpha", Version: "1", Access: "free", Results: map[string]map[string]CaseOutcome{"free-only": {"open": {Rank: 2}, "filtered": {Rank: 1}}}},
		{ID: "b", Adjudication: "accepted", Code: "fw-alpha", Version: "1", Access: "free", Results: map[string]map[string]CaseOutcome{"free-only": {"open": {Abstention: "abstained"}, "filtered": {Rank: 1}}}},
		{ID: "n", Adjudication: "accepted", Negative: true, Results: map[string]map[string]CaseOutcome{"free-only": {"open": {Abstention: "abstained"}}}},
		{ID: "u", Adjudication: "unverified", Access: "free", Results: map[string]map[string]CaseOutcome{"free-only": {"open": {Error: true}, "filtered": {}}}},
	}
	metrics := ComputeMetrics(cases, "public", map[string]int{"fw-alpha": 1})
	require.Equal(t, .5, *metrics.Modes["open"].Recall.Value)
	require.Equal(t, .25, *metrics.Modes["open"].MRR.Value)
	require.Equal(t, .75, *metrics.Modes["open"].BalancedAccuracy.Value)
	require.Equal(t, 2, metrics.Modes["open"].Recall.Denominator)
	require.Equal(t, "not_applicable", metrics.Modes["open"].CurrentVersion.Status)
	require.True(t, metrics.Filter.Pass)
	require.Equal(t, 1, metrics.Filter.Gains)
	cases[0].Results["free-only"]["filtered"] = CaseOutcome{}
	require.False(t, ComputeMetrics(cases, "public", nil).Filter.Pass)
}

func TestEvaluationMetrics_Unavailable(t *testing.T) {
	require.Equal(t, "unavailable", ComputeMetrics(nil, "public", nil).Modes["open"].Recall.Status)
	cases := []CaseResult{{Adjudication: "accepted", Access: "free", Results: map[string]map[string]CaseOutcome{"free-only": {"open": {Error: true}}}}}
	require.Equal(t, "unavailable", ComputeMetrics(cases, "public", nil).Modes["open"].Recall.Status)
}

func TestEvaluationMetrics_CurrentVersionAndCohortCaller(t *testing.T) {
	cases := []CaseResult{{Adjudication: "accepted", Code: "fw-alpha", Version: "1", Access: "licensed", Results: map[string]map[string]CaseOutcome{"full": {"open": {CurrentHits: 2, FrameworkHits: 3, Rank: 1}, "filtered": {Rank: 1}}}}}
	require.Equal(t, 0, ComputeMetrics(cases, "public", map[string]int{"fw-alpha": 2}).Modes["open"].Recall.Denominator)
	m := ComputeMetrics(cases, "licensed", map[string]int{"fw-alpha": 2})
	require.InDelta(t, 2.0/3, *m.Modes["open"].CurrentVersion.Value, 1e-12)
	require.Equal(t, "unavailable", m.Modes["open"].BalancedAccuracy.Status)
	require.Equal(t, "not_applicable", m.Modes["filtered"].BalancedAccuracy.Status)
}

func TestEvaluationMetrics_NearHitCounts(t *testing.T) {
	cases := []CaseResult{{Adjudication: "accepted", Access: "free", Results: map[string]map[string]CaseOutcome{"free-only": {"open": {NearHits: []NearHit{{Kind: "parent"}, {Kind: "parent"}, {Kind: "other_version"}}}}}}}
	raw, err := json.Marshal(ComputeMetrics(cases, "public", nil).Modes["open"])
	require.NoError(t, err)
	var value map[string]any
	require.NoError(t, json.Unmarshal(raw, &value))
	require.Equal(t, map[string]any{"parent": float64(2), "other_version": float64(1)}, value["near_hits"])
}
