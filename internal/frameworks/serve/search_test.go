package serve

import (
	"context"
	"errors"
	"math"
	"testing"

	"danny.vn/trachuan/internal/embedding"
	"github.com/stretchr/testify/require"
)

func TestSearch_ValidationRetainsProjection(t *testing.T) {
	e := testEngine()
	caller := fakeCaller()
	caller.AccessUnavailable = true
	response, err := e.Search(context.Background(), Request{Query: "access", Filters: map[string][]string{"unknown": {"value"}}}, caller)
	require.ErrorIs(t, err, ErrInvalidRequest)
	require.Equal(t, e.project(caller).version, response.AccessSetVersion)
	require.True(t, response.AccessUnavailable)
	require.Equal(t, e.release, response.Release)
	_, err = e.Search(context.Background(), Request{Query: "access"}, caller)
	require.ErrorIs(t, err, ErrUnavailable)
}

type failedQuery struct{ err error }

func (failedQuery) Recipe(context.Context) (embedding.Recipe, error) {
	return embedding.QueryRecipe1(), nil
}

func (q failedQuery) Embed(context.Context, string) (embedding.Vector, error) {
	return embedding.Vector{}, q.err
}

func TestEmbed_FixedErrors(t *testing.T) {
	for _, tc := range []struct{ source, want error }{
		{errors.New("synthetic private backend detail"), ErrUnavailable},
		{&embedding.Error{Status: 429, Code: "busy"}, ErrRateLimited},
		{&embedding.Error{Status: 413, Code: "input_too_long"}, ErrInvalidRequest},
	} {
		e := &Engine{embedder: failedQuery{tc.source}}
		_, err := e.embed(context.Background(), "quartz")
		require.ErrorIs(t, err, tc.want)
		require.Equal(t, tc.want.Error(), err.Error())
	}
}

func TestVector_RejectsInvalidRuntimeOutput(t *testing.T) {
	r := embedding.QueryRecipe1()
	values := make([]float32, r.Dimensions)
	values[0] = 1
	vector := embedding.Vector{Values: values, RecipeHash: r.Hash(), TokenCount: 4}
	require.NoError(t, validateVector(vector, r))
	for _, change := range []func(*embedding.Vector){
		func(v *embedding.Vector) { v.Values = nil },
		func(v *embedding.Vector) { v.Values = []float32{1} },
		func(v *embedding.Vector) { v.RecipeHash = "wrong" },
		func(v *embedding.Vector) { v.TokenCount = 0 },
		func(v *embedding.Vector) { v.TokenCount = 257 },
		func(v *embedding.Vector) { v.Values = make([]float32, r.Dimensions) },
		func(v *embedding.Vector) {
			v.Values = append([]float32(nil), values...)
			v.Values[0] = float32(math.NaN())
		},
	} {
		changed := vector
		change(&changed)
		require.Error(t, validateVector(changed, r))
	}
}
