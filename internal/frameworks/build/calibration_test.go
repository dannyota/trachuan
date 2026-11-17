package build

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/embedding"
	"github.com/stretchr/testify/require"
)

type calibrationAnswer struct {
	cosine *float64
	lookup bool
	err    error
}

type calibrationEngine struct{ answers map[string]calibrationAnswer }

func (e calibrationEngine) CalibrationSearch(_ context.Context, query string) (*float64, bool, error) {
	a, ok := e.answers[query]
	if !ok {
		return nil, false, errors.New("unexpected private query")
	}
	return a.cosine, a.lookup, a.err
}

func (calibrationEngine) CalibrationRecipeHash() string { return strings.Repeat("a", 64) }

func cosine(value float64) *float64 { return &value }

func calibrationCases() []GoldenCase {
	return []GoldenCase{
		{ID: "positive", Question: "private invented positive", Adjudication: "accepted", Access: "free"},
		{ID: "negative", Question: "private invented negative", Adjudication: "accepted", Negative: true},
	}
}

func TestCalibration_ThresholdAndProvenance(t *testing.T) {
	cases := calibrationCases()
	cases = append(cases,
		GoldenCase{ID: "licensed", Question: "never execute", Adjudication: "accepted", Access: "licensed"},
		GoldenCase{ID: "unverified", Question: "never execute", Adjudication: "unverified", Access: "free"},
		GoldenCase{ID: "rejected", Question: "never execute", Adjudication: "rejected", Negative: true},
		GoldenCase{ID: "outside", Question: "never execute", Adjudication: "out_of_release", Access: "free"},
	)
	e := calibrationEngine{map[string]calibrationAnswer{
		"private invented positive": {cosine: cosine(.8)}, "private invented negative": {cosine: cosine(.2)},
	}}
	r, err := CalibrateCases(context.Background(), e, cases, strings.Repeat("b", 64))
	require.NoError(t, err)
	require.Equal(t, .5, r.Threshold)
	require.Len(t, r.Candidates, 3)
	require.Len(t, r.Scores, 2)
	require.Equal(t, strings.Repeat("b", 64), r.CaseFileSHA256)
	require.Equal(t, strings.Repeat("a", 64), r.EngineRecipeSHA256)
	require.Equal(t, embedding.QueryRecipe1().Hash(), r.QueryEmbedderRecipeSHA256)
	encoded, err := json.Marshal(r)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "private invented")
	recipe := measuredRetrievalRecipe(r.Threshold, digest(encoded))
	require.Equal(t, "measured", recipe["status"])
	require.Equal(t, bootstrapRetrievalRecipe()["rrf"], recipe["rrf"])
	a := recipe["abstention"].(map[string]any)
	require.Equal(t, .5, a["threshold"])
	require.Equal(t, digest(encoded), a["calibration_sha256"])
}

func TestCalibration_EmptyLookupAndTies(t *testing.T) {
	for _, tc := range []struct {
		name               string
		positive, negative calibrationAnswer
		threshold          float64
		lookups            int
	}{
		{name: "tie", positive: calibrationAnswer{cosine: cosine(.2)}, negative: calibrationAnswer{cosine: cosine(.8)}, threshold: -1},
		{name: "empty positive", negative: calibrationAnswer{cosine: cosine(.2)}, threshold: 2},
		{name: "empty negative", positive: calibrationAnswer{cosine: cosine(.8)}, threshold: -1},
		{name: "lookup positive", positive: calibrationAnswer{lookup: true}, negative: calibrationAnswer{cosine: cosine(.2)}, threshold: 2, lookups: 1},
		{name: "lookup negative", positive: calibrationAnswer{cosine: cosine(.8)}, negative: calibrationAnswer{lookup: true}, threshold: -1, lookups: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := calibrationEngine{map[string]calibrationAnswer{"private invented positive": tc.positive, "private invented negative": tc.negative}}
			r, err := CalibrateCases(context.Background(), e, calibrationCases(), strings.Repeat("b", 64))
			require.NoError(t, err)
			require.Equal(t, tc.threshold, r.Threshold)
			require.Equal(t, tc.lookups, r.LookupCount)
		})
	}
}

func TestCalibration_FailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer calibrationAnswer
	}{
		{"NaN", calibrationAnswer{cosine: cosine(math.NaN())}},
		{"infinity", calibrationAnswer{cosine: cosine(math.Inf(1))}},
		{"execution", calibrationAnswer{err: errors.New("private invented question and body")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := calibrationEngine{map[string]calibrationAnswer{"private invented positive": tc.answer}}
			_, err := CalibrateCases(context.Background(), e, calibrationCases(), strings.Repeat("b", 64))
			require.ErrorContains(t, err, "positive")
			require.NotContains(t, err.Error(), "private invented")
			require.Contains(t, hide("build database", err).Error(), "positive")
		})
	}
	for _, cases := range [][]GoldenCase{nil, calibrationCases()[:1], calibrationCases()[1:]} {
		_, err := CalibrateCases(context.Background(), calibrationEngine{}, cases, strings.Repeat("b", 64))
		require.ErrorContains(t, err, "missing calibration class")
	}
}
