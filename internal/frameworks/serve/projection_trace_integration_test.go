//go:build integration

package serve

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

type openingTraceKey struct{}

type queryTrace struct {
	mu      sync.Mutex
	reads   []queryRead
	classes map[string]relationClass
}

func (q *queryTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	q.mu.Lock()
	defer q.mu.Unlock()
	r := queryRead{sql: data.SQL, opening: ctx.Value(openingTraceKey{}) == true}
	if len(data.Args) > 0 {
		if ids, ok := data.Args[0].([]int64); ok {
			r.versions = slices.Clone(ids)
			r.hasVersions = true
		}
	}
	if projectionRules()[normalizeSQL(data.SQL)].kind == "gaps" && len(data.Args) > 1 {
		if ids, ok := data.Args[1].([]int64); ok {
			r.textGaps = slices.Clone(ids)
			r.hasTextGaps = true
		}
	}
	q.reads = append(q.reads, r)
	return ctx
}

func (*queryTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (q *queryTrace) reset() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.reads = nil
}

func (q *queryTrace) snapshot() []queryRead {
	q.mu.Lock()
	defer q.mu.Unlock()
	return slices.Clone(q.reads)
}

func (q *queryTrace) assertProjection(t *testing.T, readable []int64) {
	t.Helper()
	require.NoError(t, q.projectionError(readable))
}

func (q *queryTrace) assertSearchScan(t *testing.T, p projection) {
	t.Helper()
	q.assertProjection(t, p.readable)
	for _, index := range []struct {
		sql      string
		versions []int64
	}{{bodyDenseSQL, p.readable}, {publicDenseSQL, p.reduced}} {
		count := 0
		for _, read := range q.snapshot() {
			if normalizeSQL(read.sql) == normalizeSQL(index.sql) {
				count++
				require.True(t, read.hasVersions)
				require.Equal(t, index.versions, read.versions)
			}
		}
		want := 0
		if len(index.versions) > 0 {
			want = 1
		}
		require.Equal(t, want, count)
	}
}

func (q *queryTrace) projectionError(readable []int64) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return checkProjection(q.reads, q.classes, readable)
}
