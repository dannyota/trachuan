//go:build integration

package evaluate

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInvariant_SearchPreservesSQLCause(t *testing.T) {
	ctx := context.Background()
	conn, req := evaluationFixture(t)
	engine, closeEngine, err := fixtureOpener(ctx, conn, []byte(strings.Repeat("k", 32)))
	require.NoError(t, err)
	defer closeEngine()
	_, err = conn.Exec(ctx, `ALTER TABLE body_posting RENAME TO missing_body_posting`)
	require.NoError(t, err)
	_, _, err = observeSearch(ctx, engine, caseRequest(req.Cases[0], "open"), freeCaller(), req.Cases[0], "open", 0)
	require.Error(t, err)
	failure := failedInvariant("free-only", "observe_mutated", err)
	cause := failure.Failures[0].Cause
	require.Equal(t, "42P01", cause.SQLState)
	require.Equal(t, "*pgconn.PgError", cause.GoType)
	raw, err := json.Marshal(failure)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "relation")
	require.NotContains(t, string(raw), "needle")
}
