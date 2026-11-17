package evaluate

import (
	"context"
	"errors"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/frameworks/build"
	"danny.vn/trachuan/internal/frameworks/serve"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

type workloadLeak struct {
	workloadEngine
	class            string
	executionFailure bool
}

func (e workloadLeak) Search(ctx context.Context, r serve.Request, caller serve.Caller) (serve.Response, error) {
	response, err := e.workloadEngine.Search(ctx, r, caller)
	for _, code := range []string{"fw-alpha", "fw-gamma"} {
		if e.state.changed(code, e.class) && !caller.Readable(code, "licensed") && r.Cursor == "2" {
			if e.executionFailure {
				return response, &pgconn.PgError{Code: "23514", ConstraintName: "invented_check", Message: "invented private message"}
			}
			response.Message = "invented changed response"
		}
	}
	return response, err
}
func countedRestores(copies invariantCopies) (invariantCopies, *int) {
	restores := 0
	original := copies.restore
	copies.restore = func(ctx context.Context, req build.DatabaseRequest, suffix string, temporary bool, use func(context.Context, *pgx.Conn) error) error {
		restores++
		return original(ctx, req, suffix, temporary, use)
	}
	return copies, &restores
}
func workloadInvariant(t *testing.T, req EvaluationRequest, copies invariantCopies) InvariantResult {
	t.Helper()
	ctx := context.Background()
	base, closeEngine, err := copies.open(ctx, nil, nil)
	require.NoError(t, err)
	defer closeEngine()
	status, err := base.CorpusStatus(ctx, serve.StatusRequest{}, fullCaller())
	require.NoError(t, err)
	return checkInvariant(ctx, req, base, copies, nil, status)
}
func TestInvariant_AttributesFinalMismatchByReplayingStages(t *testing.T) {
	for _, class := range privateClasses() {
		t.Run(class, func(t *testing.T) {
			req := workloadRequest()
			counts := &workloadCounts{}
			copies := workloadCopies(counts, NewQueryCache(workloadQuery{counts}), req.Cases)
			original := copies.open
			copies.open = func(ctx context.Context, conn *pgx.Conn, key []byte) (Engine, func(), error) {
				engine, closeEngine, err := original(ctx, conn, key)
				if err != nil {
					return nil, nil, err
				}
				return workloadLeak{workloadEngine: engine.(workloadEngine), class: class}, closeEngine, nil
			}
			copies, restores := countedRestores(copies)
			result := workloadInvariant(t, req, copies)
			require.False(t, result.Pass)
			require.Equal(t, 6, *restores)
			require.Equal(t, 31, counts.opens)
			for _, caller := range result.Callers {
				require.False(t, caller.Classes[class].Pass)
				require.True(t, caller.Classes[class].PositiveControl)
				require.NotEmpty(t, caller.Failures)
				for _, failure := range caller.Failures {
					require.Equal(t, class, failure.Class)
					require.Equal(t, "search", failure.Operation)
					require.Equal(t, 2, failure.Page)
					require.NotEmpty(t, failure.Cause.GoType)
				}
			}
		})
	}
}
func TestInvariant_PositiveFailureDoesNotReplayReducedWorkload(t *testing.T) {
	req := workloadRequest()
	counts := &workloadCounts{}
	copies := workloadCopies(counts, NewQueryCache(workloadQuery{counts}), req.Cases)
	original := copies.mutate
	copies.mutate = func(ctx context.Context, conn *pgx.Conn, caller serve.Caller, class string) error {
		if class == "body_index" {
			return nil
		}
		return original(ctx, conn, caller, class)
	}
	copies, restores := countedRestores(copies)
	result := workloadInvariant(t, req, copies)
	require.False(t, result.Pass)
	require.Equal(t, 3, *restores)
	require.Equal(t, 16, counts.opens)
	for _, caller := range result.Callers {
		require.False(t, caller.Classes["body_index"].PositiveControl)
		require.Len(t, caller.Failures, 1)
		require.Equal(t, "positive_control", caller.Failures[0].Operation)
	}
}
func TestInvariant_AttributesExecutionFailureWithSafeCause(t *testing.T) {
	req := workloadRequest()
	counts := &workloadCounts{}
	copies := workloadCopies(counts, NewQueryCache(workloadQuery{counts}), req.Cases)
	original := copies.open
	copies.open = func(ctx context.Context, conn *pgx.Conn, key []byte) (Engine, func(), error) {
		engine, closeEngine, err := original(ctx, conn, key)
		if err != nil {
			return nil, nil, err
		}
		return workloadLeak{workloadEngine: engine.(workloadEngine), class: "text_gaps", executionFailure: true}, closeEngine, nil
	}
	result := workloadInvariant(t, req, copies)
	require.False(t, result.Pass)
	for _, caller := range result.Callers {
		require.NotEmpty(t, caller.Failures)
		for _, failure := range caller.Failures {
			require.Equal(t, "text_gaps", failure.Class)
			require.Equal(t, "23514", failure.Cause.SQLState)
			require.Equal(t, "invented_check", failure.Cause.Constraint)
		}
	}
}

func TestInvariant_FinalCauseSurvivesAttributionRestoreFailure(t *testing.T) {
	req := workloadRequest()
	counts := &workloadCounts{}
	copies := workloadCopies(counts, NewQueryCache(workloadQuery{counts}), req.Cases)
	originalOpen, originalRestore := copies.open, copies.restore
	copies.open = func(ctx context.Context, conn *pgx.Conn, key []byte) (Engine, func(), error) {
		engine, closeEngine, err := originalOpen(ctx, conn, key)
		if err != nil {
			return nil, nil, err
		}
		return workloadLeak{workloadEngine: engine.(workloadEngine), class: "text_gaps", executionFailure: true}, closeEngine, nil
	}
	copies.restore = func(ctx context.Context, req build.DatabaseRequest, suffix string, temporary bool, use func(context.Context, *pgx.Conn) error) error {
		if strings.HasPrefix(suffix, "_eval_mut_10") {
			return errors.New("invented private restore message")
		}
		return originalRestore(ctx, req, suffix, temporary, use)
	}
	result := workloadInvariant(t, req, copies)
	require.False(t, result.Pass)
	for _, caller := range result.Callers {
		sqlCause, restoreCause := false, false
		for _, failure := range caller.Failures {
			sqlCause = sqlCause || failure.Cause.SQLState == "23514"
			restoreCause = restoreCause || failure.Operation == "restore_attribution"
		}
		require.True(t, sqlCause)
		require.True(t, restoreCause)
	}
}
