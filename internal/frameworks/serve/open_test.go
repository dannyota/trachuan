package serve

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecipe_InvalidParametersFailClosed(t *testing.T) {
	require.NoError(t, rankRecipe().validate())
	for _, change := range []func(*recipe){
		func(r *recipe) { r.Version = 0 }, func(r *recipe) { r.DenseDepth = 0 },
		func(r *recipe) { r.Status = "" },
		func(r *recipe) { r.LexicalDepth = -1 }, func(r *recipe) { r.RRF.K = 0 },
		func(r *recipe) { r.RRF.DenseWeight = math.NaN() },
		func(r *recipe) { r.RRF.LexicalWeight = -1 },
		func(r *recipe) { r.Abstention.Threshold = nil },
		func(r *recipe) { v := math.Inf(1); r.Abstention.Threshold = &v },
		func(r *recipe) { r.Abstention.Statistic = "first-group-best-member-cosine" },
		func(r *recipe) { r.Abstention.Status = "" },
		func(r *recipe) { r.Source.Release = "" },
		func(r *recipe) { r.Source.CalibrationSHA256 = "bad" },
	} {
		r := rankRecipe()
		change(&r)
		require.ErrorIs(t, r.validate(), ErrUnavailable)
	}
	var r recipe
	require.Error(t, decodeRecipe([]byte(`{"version":1,"unexpected":true}`), &r))
	encoded, err := json.Marshal(rankRecipe())
	require.NoError(t, err)
	require.NoError(t, decodeRecipe(encoded, &r))
	require.NoError(t, r.validate())
}

func TestRecipe_RefusesPreFusionFilters(t *testing.T) {
	r := rankRecipe()
	r.Version = 1
	require.ErrorIs(t, r.validate(), ErrUnavailable)
	r.Version = 2
	require.NoError(t, r.validate())
}

func TestOpen_MissingDependenciesAreUnavailable(t *testing.T) {
	_, err := Open(context.Background(), nil, nil, Options{})
	require.ErrorIs(t, err, ErrUnavailable)
	require.Equal(t, "unavailable", err.Error())
}

func TestRelease_RejectsWrongKindSchemaAndUnsealed(t *testing.T) {
	require.NoError(t, validateIdentity("frameworks", "20990101", 1, true))
	for _, tc := range []struct {
		kind, rel string
		schema    int
		sealed    bool
	}{
		{"law", "20990101", 1, true}, {"frameworks", "", 1, true},
		{"frameworks", "../20990101", 1, true}, {"frameworks", "20990101", 2, true},
		{"frameworks", "20990101", 1, false},
	} {
		require.ErrorIs(t, validateIdentity(tc.kind, tc.rel, tc.schema, tc.sealed), ErrUnavailable)
	}
}

func TestStep_RequiresSupportedChunkRepresentation(t *testing.T) {
	require.True(t, validStep([]byte(`{"name":"chunk","version":1,"parameters":{"schema":"chunks/1","limit":512,"prefix_format_version":1}}`), "chunk"))
	for _, raw := range []string{
		`{}`, `null`, `{"name":"chunk","version":1,"parameters":{}}`,
		`{"name":"chunk","version":2,"parameters":{"schema":"chunks/1","limit":512,"prefix_format_version":1}}`,
		`{"name":"chunk","version":1,"parameters":{"schema":"chunks/2","limit":512,"prefix_format_version":1}}`,
	} {
		require.False(t, validStep([]byte(raw), "chunk"))
	}
}
