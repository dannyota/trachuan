//go:build integration

package serve

import (
	"context"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
	"github.com/stretchr/testify/require"
)

func assertVersionScan(t *testing.T, e *Engine) {
	t.Helper()
	ctx := context.Background()
	tx, err := e.db.Begin(ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, tx.Rollback(ctx)) }()
	_, err = tx.Exec(ctx, transactionSettingsSQL)
	require.NoError(t, err)
	rows, err := tx.Query(ctx, "EXPLAIN "+bodyDenseSQL, []int64{3}, retrieval.DenseVector(syntheticVector("quartz")))
	require.NoError(t, err)
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		lines = append(lines, line)
	}
	require.NoError(t, rows.Err())
	plan := strings.Join(lines, "\n")
	require.Contains(t, plan, "body_chunk_version")
	require.Contains(t, plan, "Index Cond:")
	require.NotContains(t, plan, "Seq Scan on body_chunk")
}
