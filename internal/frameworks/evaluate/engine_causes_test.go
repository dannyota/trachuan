package evaluate

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"danny.vn/trachuan/internal/frameworks/build"
	"danny.vn/trachuan/internal/frameworks/serve"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestCauseTracer_RecordsSafeUnderlyingFailure(t *testing.T) {
	original := &pgconn.PgError{Code: "57014", TableName: "body_chunk", Message: "invented private message", Detail: "invented private detail"}
	_, err := captureOperation(context.Background(), func(ctx context.Context) (int, error) {
		CauseTracer{}.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{Err: original})
		CauseTracer{}.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{Err: errors.New("later failure")})
		return 0, serve.ErrUnavailable
	})
	require.ErrorIs(t, err, serve.ErrUnavailable)
	cause := build.IdentifyFailure(err)
	require.Equal(t, "57014", cause.SQLState)
	raw, marshalErr := json.Marshal(cause)
	require.NoError(t, marshalErr)
	require.NotContains(t, string(raw), "private")
	require.NotContains(t, err.Error(), "private")
}
func TestCauseTracer_IgnoresRolledBackSuccessfulOperation(t *testing.T) {
	_, err := captureOperation(context.Background(), func(ctx context.Context) (int, error) { recordCause(ctx, errors.New("cleanup failure")); return 1, nil })
	require.NoError(t, err)
}
