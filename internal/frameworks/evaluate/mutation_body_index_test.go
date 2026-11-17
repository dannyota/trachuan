package evaluate

import (
	"context"
	"testing"

	"danny.vn/trachuan/internal/frameworks/serve"
	"github.com/stretchr/testify/require"
)

type chunkCountFixture struct {
	invariantFixture
	chunks int
}

func (f chunkCountFixture) CorpusStatus(ctx context.Context, r serve.StatusRequest, caller serve.Caller) (serve.StatusResponse, error) {
	response, err := f.invariantFixture.CorpusStatus(ctx, r, caller)
	response.Totals.Controls += f.chunks
	return response, err
}
func TestInvariant_DetectsInjectedChunkCountLeak(t *testing.T) {
	req := EvaluationRequest{Cases: []GoldenCase{{ID: "p", Question: "invented query", Code: "fw-alpha", Adjudication: "accepted"}}}
	before := chunkCountFixture{chunks: 1}
	after := chunkCountFixture{invariantFixture: invariantFixture{changed: "yes"}, chunks: 2}
	a, err := observe(context.Background(), before, req, freeCaller())
	require.NoError(t, err)
	b, err := observe(context.Background(), after, req, freeCaller())
	require.NoError(t, err)
	fullA, err := observe(context.Background(), before, req, fullCaller())
	require.NoError(t, err)
	fullB, err := observe(context.Background(), after, req, fullCaller())
	require.NoError(t, err)
	result := compareMutationClass("free-only", "body_index", a, b, fullA, fullB)
	require.False(t, result.Pass)
	require.True(t, result.Classes["body_index"].PositiveControl)
	require.Contains(t, result.Failures, InvariantFailure{Operation: "corpus_status", Class: "body_index", Repeat: 0})
}
