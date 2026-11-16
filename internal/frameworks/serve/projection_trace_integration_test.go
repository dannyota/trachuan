//go:build integration

package serve

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

type queryRead struct {
	sql      string
	versions []int64
	textGaps []int64
}

type queryTrace struct {
	mu    sync.Mutex
	reads []queryRead
}

func (q *queryTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	q.mu.Lock()
	defer q.mu.Unlock()
	r := queryRead{sql: data.SQL}
	if len(data.Args) > 0 {
		if ids, ok := data.Args[0].([]int64); ok {
			r.versions = slices.Clone(ids)
		}
	}
	if strings.Contains(data.SQL, "FROM quality_gap") && len(data.Args) > 1 {
		if ids, ok := data.Args[1].([]int64); ok {
			r.textGaps = slices.Clone(ids)
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
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, read := range q.reads {
		private := strings.Contains(read.sql, "body_chunk") || strings.Contains(read.sql, "body_posting") || strings.Contains(read.sql, "control_text")
		if private {
			require.NotEmpty(t, read.versions, "private read without a version filter")
			require.Contains(t, read.sql, "version_id=ANY($1::bigint[])")
			for _, id := range read.versions {
				require.Contains(t, readable, id, "private structure read for an unreadable version")
			}
		}
		if strings.Contains(read.sql, "FROM quality_gap") {
			require.Contains(t, read.sql, "g.class='public' OR (g.class='text' AND g.version_id=ANY($2::bigint[]))")
			for _, id := range read.textGaps {
				require.Contains(t, readable, id, "text gap read for an unreadable version")
			}
		}
	}
}
