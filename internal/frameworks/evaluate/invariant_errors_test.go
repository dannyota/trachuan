package evaluate

import (
	"context"
	"testing"

	"danny.vn/trachuan/internal/frameworks/serve"
	"github.com/stretchr/testify/require"
)

type failedSearchFixture struct{ invariantFixture }

func (f failedSearchFixture) Search(ctx context.Context, r serve.Request, caller serve.Caller) (serve.Response, error) {
	if caller.Readable("fw-alpha", "licensed") {
		return serve.Response{}, serve.ErrUnavailable
	}
	return f.invariantFixture.Search(ctx, r, caller)
}
func TestInvariant_FailedFullSearchCannotSatisfyPositiveControl(t *testing.T) {
	req := EvaluationRequest{Cases: []GoldenCase{{ID: "p", Question: "invented query", Code: "fw-alpha", Adjudication: "accepted"}}}
	before, err := observePositive(context.Background(), invariantFixture{}, req, "body_index")
	require.NoError(t, err)
	after, afterErr := observePositive(context.Background(), failedSearchFixture{}, req, "body_index")
	result := positiveResult("free-only", "body_index", before, after, nil, afterErr)
	require.Error(t, afterErr)
	require.False(t, result.Pass)
	require.False(t, result.Classes["body_index"].PositiveControl)
	require.NotEmpty(t, result.Failures[0].Cause.GoType)

}
func TestInvariant_DiagnosticFailureIsNotPositiveEvidence(t *testing.T) {
	req := EvaluationRequest{Cases: []GoldenCase{{ID: "u", Question: "invented diagnostic", Code: "fw-alpha", Adjudication: "unverified"}}}
	reduced, err := observe(context.Background(), invariantFixture{}, req, freeCaller())
	require.NoError(t, err)
	fullBefore, err := observe(context.Background(), invariantFixture{}, req, fullCaller())
	require.NoError(t, err)
	fullAfter, err := observe(context.Background(), failedSearchFixture{}, req, fullCaller())
	require.NoError(t, err)
	result := compareMutationClass("free-only", "body_index", reduced, reduced, fullBefore, fullAfter)
	require.False(t, result.Pass)
	require.False(t, result.Classes["body_index"].PositiveControl)
}

type diagnosticReplayFixture struct {
	invariantFixture
	calls     int
	failFirst bool
}

func (f *diagnosticReplayFixture) Search(ctx context.Context, r serve.Request, caller serve.Caller) (serve.Response, error) {
	f.calls++
	if (f.failFirst && f.calls == 1) || (!f.failFirst && f.calls == 2) {
		return serve.Response{}, serve.ErrUnavailable
	}
	return f.invariantFixture.Search(ctx, r, caller)
}
func TestInvariant_DiagnosticReplayErrorsDoNotGate(t *testing.T) {
	req := EvaluationRequest{Cases: []GoldenCase{{ID: "u", Question: "invented diagnostic", Code: "fw-alpha", Adjudication: "unverified"}}}
	for _, first := range []bool{false, true} {
		_, err := observe(context.Background(), &diagnosticReplayFixture{failFirst: first}, req, fullCaller())
		require.NoError(t, err)
	}
}
