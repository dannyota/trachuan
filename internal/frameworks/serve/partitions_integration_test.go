//go:build integration

package serve

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSearch_ReducedCallerNeverTouchesLicensedBodyPartitions(t *testing.T) {
	e, trace := buildFixture(t, false)
	ctx := context.Background()
	r := Request{Query: "quartz", Limit: 50}
	reduced, err := e.Search(ctx, r, fakeCaller())
	require.NoError(t, err)
	full, err := e.Search(ctx, r, fakeCaller("*"))
	require.NoError(t, err)
	require.NotEmpty(t, full.Hits)
	for _, v := range e.versions {
		if v.info.Access == "licensed" {
			e.partitions[v.id].body = indexPartition{}
		}
	}
	trace.reset()
	after, err := e.Search(ctx, r, fakeCaller())
	require.NoError(t, err)
	require.Equal(t, responseBytes(t, reduced, nil), responseBytes(t, after, nil))
	trace.assertProjection(t, e.project(fakeCaller()).readable)
	_, err = e.Search(ctx, r, fakeCaller("*"))
	require.ErrorIs(t, err, ErrUnavailable)
}

func TestOpen_LoadsEveryPartitionWithoutPrivateText(t *testing.T) {
	e, trace := buildOperationsFixture(t, false)
	trace.reset()
	reopened, err := Open(context.WithValue(context.Background(), openingTraceKey{}, true), e.db, syntheticQuery{}, Options{CursorKey: e.key})
	require.NoError(t, err)
	require.Len(t, trace.snapshot(), 10)
	trace.assertProjection(t, nil)
	for _, v := range reopened.versions {
		part := reopened.partitions[v.id]
		require.NotEmpty(t, part.controls)
		require.NotEmpty(t, part.body.chunks)
		require.NotEmpty(t, part.public.chunks)
		require.Positive(t, part.body.stats.N)
		require.NotEmpty(t, part.body.stats.DF)
	}
	require.Equal(t, 206, reopened.partitions[2].citations["A-1"].mappings)
	require.Equal(t, Counts{Controls: 4, EdgesResolved: 205}, reopened.partitions[2].counts)
	require.Equal(t, Counts{Controls: 4, EdgesResolved: 1}, reopened.partitions[3].counts)
	require.Equal(t, Counts{Controls: 4, EdgesUnresolved: 1}, reopened.partitions[4].counts)
}

func TestOpen_EnforcesPartitionBudgetWhileLoading(t *testing.T) {
	e, trace := buildFixture(t, false)
	e.partitionBytes = partitionMemoryLimit
	trace.reset()
	require.ErrorIs(t, e.loadPartitions(context.Background()), ErrUnavailable)
	require.Empty(t, trace.snapshot())
}
