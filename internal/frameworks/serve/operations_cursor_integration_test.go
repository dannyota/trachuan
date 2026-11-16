//go:build integration

package serve

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOperations_ContinuationBindings(t *testing.T) {
	e, trace := buildOperationsFixture(t, false)
	ctx := context.Background()
	caller := fakeCaller()
	first, err := e.Mappings(ctx, MappingsRequest{Handle: "fw:fw-alpha:2#A-1", Limit: 1}, caller)
	require.NoError(t, err)
	require.NotEmpty(t, first.NextCursor)
	for _, request := range []MappingsRequest{
		{Handle: "fw:fw-alpha:2#A-1", Limit: 2, Cursor: first.NextCursor},
		{Handle: "fw:fw-beta:1#A-1", Limit: 1, Cursor: first.NextCursor},
		{Handle: "fw:fw-alpha:2#A-1", Limit: 1, Relations: []string{"related"}, Cursor: first.NextCursor},
	} {
		_, err = e.Mappings(ctx, request, caller)
		require.ErrorIs(t, err, ErrInvalidRequest)
	}
	_, err = e.Mappings(ctx, MappingsRequest{Handle: "fw:fw-alpha:2#A-1", Limit: 1, Cursor: first.NextCursor}, fakeCaller("fw-alpha"))
	require.ErrorIs(t, err, ErrCursorExpired)
	gaps, err := e.QualityGaps(ctx, GapsRequest{}, caller)
	require.NoError(t, err)
	require.NotEmpty(t, gaps.NextCursor)
	_, err = e.QualityGaps(ctx, GapsRequest{Cursor: gaps.NextCursor}, fakeCaller("fw-alpha"))
	require.ErrorIs(t, err, ErrCursorExpired)
	_, err = e.QualityGaps(ctx, GapsRequest{Kind: "mapping_dropped", Cursor: gaps.NextCursor}, caller)
	require.ErrorIs(t, err, ErrInvalidRequest)
	_, err = e.QualityGaps(ctx, GapsRequest{Versions: []string{"fw:fw-alpha:2"}, Cursor: gaps.NextCursor}, caller)
	require.ErrorIs(t, err, ErrInvalidRequest)
	_, err = e.QualityGaps(ctx, GapsRequest{Cursor: first.NextCursor}, caller)
	require.ErrorIs(t, err, ErrInvalidRequest)
	read, err := e.Read(ctx, ReadRequest{Items: []ReadItem{{Handle: "fw:fw-alpha:2#A-1"}}}, caller)
	require.NoError(t, err)
	require.NotEmpty(t, read.Items[0].Next)
	continued, err := e.Read(ctx, ReadRequest{Items: []ReadItem{{Next: read.Items[0].Next}, {Handle: "fw:fw-beta:1"}}}, fakeCaller("fw-alpha"))
	require.NoError(t, err)
	require.Equal(t, "invalid_next", continued.Items[0].Error)
	require.True(t, continued.Items[1].Complete)
	c, err := e.unpackContinuation(read.Items[0].Next)
	require.NoError(t, err)
	c.Release = "20990102"
	continued, err = e.Read(ctx, ReadRequest{Items: []ReadItem{{Next: e.encodeContinuation(c)}, {Handle: "fw:fw-beta:1"}}}, caller)
	require.NoError(t, err)
	require.Equal(t, "release_expired", continued.Items[0].Error)
	require.True(t, continued.Items[1].Complete)
	trace.reset()
	continued, err = e.Read(ctx, ReadRequest{Items: []ReadItem{{Next: first.NextCursor}, {Handle: "fw:fw-beta:1"}}}, caller)
	require.NoError(t, err)
	require.Equal(t, "invalid_next", continued.Items[0].Error)
	require.True(t, continued.Items[1].Complete)
	trace.assertProjection(t, e.project(caller).readable)
}
