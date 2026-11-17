package evaluate

import (
	"context"
	"errors"
	"testing"

	"danny.vn/trachuan/internal/embedding"
	"github.com/stretchr/testify/require"
)

type cacheQueryFixture struct {
	calls, recipes int
	err            error
}

func (q *cacheQueryFixture) Recipe(context.Context) (embedding.Recipe, error) {
	q.recipes++
	return embedding.QueryRecipe1(), q.err
}
func (q *cacheQueryFixture) Embed(context.Context, string) (embedding.Vector, error) {
	q.calls++
	return embedding.Vector{Values: []float32{1}, TokenCount: 1}, q.err
}
func TestQueryCache_DistinctQueriesAndRecipe(t *testing.T) {
	ctx := context.Background()
	source := &cacheQueryFixture{}
	cache := NewQueryCache(source)
	require.NoError(t, cache.Prepare(ctx, []GoldenCase{{Adjudication: "accepted", Question: "invented one"}, {Adjudication: "accepted", Question: "invented two"}, {Adjudication: "accepted", Question: "invented one"}}))
	for range 3 {
		v, err := cache.Embed(ctx, "invented one")
		require.NoError(t, err)
		require.Equal(t, float32(1), v.Values[0])
		v.Values[0] = 0
		_, err = cache.Recipe(ctx)
		require.NoError(t, err)
	}
	require.Equal(t, 2, source.calls)
	require.Equal(t, 1, source.recipes)
}
func TestQueryCache_ErrorsAndCancellation(t *testing.T) {
	ctx := context.Background()
	source := &cacheQueryFixture{err: errors.New("invented private message")}
	cache := NewQueryCache(source)
	for range 3 {
		_, err := cache.Embed(ctx, "invented query")
		require.ErrorIs(t, err, source.err)
	}
	require.Equal(t, 1, source.calls)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	require.ErrorIs(t, cache.Prepare(cancelled, []GoldenCase{{Adjudication: "accepted", Question: "another invented query"}}), context.Canceled)
	require.Equal(t, 1, source.calls)
}
