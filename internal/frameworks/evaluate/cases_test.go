package evaluate

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"danny.vn/trachuan/internal/frameworks/serve"
	"github.com/stretchr/testify/require"
)

func TestCaseOutcome_ExactTargetAndNearHits(t *testing.T) {
	parent := "P"
	child := "A"
	controls := map[string]Control{
		"fw:fw-alpha:1#A": {Code: "fw-alpha", Version: "1", Citation: "A", Parent: &parent},
		"fw:fw-alpha:1#P": {Code: "fw-alpha", Version: "1", Citation: "P"},
		"fw:fw-alpha:1#C": {Code: "fw-alpha", Version: "1", Citation: "C", Parent: &child},
		"fw:fw-alpha:2#A": {Code: "fw-alpha", Version: "2", Citation: "A"},
	}
	c := GoldenCase{Code: "fw-alpha", Version: "1", Target: "fw:fw-alpha:1#A"}
	response := serve.Response{Hits: []serve.Hit{{Handle: "fw:fw-alpha:2#A"}, {Handle: "fw:fw-alpha:1#P"}, {Handle: "fw:fw-alpha:1#C"}, {}, {}, {}, {}, {Handle: c.Target}}}
	outcome := caseOutcome(c, response, nil, controls)
	require.Equal(t, 8, outcome.Rank)
	require.Len(t, outcome.NearHits, 3)
	require.Equal(t, 3, outcome.CurrentHits)
	require.Equal(t, 4, outcome.FrameworkHits)
	response.Hits = append([]serve.Hit{{}}, response.Hits...)
	require.True(t, caseOutcome(c, response, nil, controls).Miss)
	require.True(t, caseOutcome(c, response, errors.New("invented private error"), controls).Error)
}
func TestScoreCases_ReportHasNoTextAndUnverifiedIsDiagnostic(t *testing.T) {
	req := EvaluationRequest{Cases: []GoldenCase{
		{ID: "a", Question: "question must never appear", Code: "fw-alpha", Version: "1", Target: "fw:fw-alpha:1#A", Access: "free", Adjudication: "accepted"},
		{ID: "u", Question: "unverified secret question", Code: "fw-alpha", Version: "1", Target: "fw:fw-alpha:1#A", Access: "licensed", Adjudication: "unverified"},
		{ID: "n", Question: "negative secret question", Negative: true, Adjudication: "accepted"},
	}}
	rows := scoreCases(context.Background(), invariantFixture{}, req)
	require.Len(t, rows, 3)
	require.Equal(t, 1, rows[0].Results["free-only"]["open"].Rank)
	require.Equal(t, "not_applicable", rows[1].Filter["licensed"])
	require.Contains(t, rows[1].Results, "full")
	require.NotContains(t, rows[2].Results["full"], "filtered")
	raw, err := json.Marshal(rows)
	require.NoError(t, err)
	for _, text := range []string{"question must never appear", "unverified secret question", "negative secret question", "original", "changed"} {
		require.NotContains(t, string(raw), text)
	}
}
