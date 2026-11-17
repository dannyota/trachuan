package evaluate

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestInvariantFailure_RecordsSafeCause(t *testing.T) {
	cause := &pgconn.PgError{Code: "23514", TableName: "body_chunk", ColumnName: "token_count", ConstraintName: "body_chunk_token_count_check", Message: "invented private control", Detail: "invented private question"}
	result := failedInvariant("free-only", "observe_mutated", &caseFailure{id: "synthetic-case", operation: "invariant search failed", cause: fmt.Errorf("hidden text: %w", cause)})
	raw, err := json.Marshal(result.Failures[0])
	require.NoError(t, err)
	var value map[string]any
	require.NoError(t, json.Unmarshal(raw, &value))
	require.Equal(t, map[string]any{"sqlstate": "23514", "table": "body_chunk", "column": "token_count", "constraint": "body_chunk_token_count_check", "go_type": "*pgconn.PgError"}, value["cause"])
	require.NotContains(t, string(raw), "invented private")
	require.NotContains(t, string(raw), "hidden text")
}
func TestInvariantFailure_RecordsGoCauseType(t *testing.T) {
	result := failedInvariant("free-only", "restore", fmt.Errorf("private context: %w", errors.New("invented private control")))
	raw, err := json.Marshal(result.Failures[0])
	require.NoError(t, err)
	var value map[string]any
	require.NoError(t, json.Unmarshal(raw, &value))
	require.Equal(t, map[string]any{"go_type": "*errors.errorString"}, value["cause"])
	require.NotContains(t, string(raw), "private")
}
