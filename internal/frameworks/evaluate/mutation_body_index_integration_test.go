//go:build integration

package evaluate

import (
	"context"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/frameworks/build"
	"danny.vn/trachuan/internal/frameworks/serve"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

type chunkCountLeakEngine struct {
	Engine
	conn *pgx.Conn
}

func (e chunkCountLeakEngine) CorpusStatus(ctx context.Context, r serve.StatusRequest, caller serve.Caller) (serve.StatusResponse, error) {
	response, err := e.Engine.CorpusStatus(ctx, r, caller)
	if err != nil {
		return response, err
	}
	var chunks int
	err = e.conn.QueryRow(ctx, `SELECT count(*) FROM body_chunk b JOIN framework_version v ON v.id=b.version_id WHERE v.access='licensed'`).Scan(&chunks)
	response.Totals.Controls += chunks
	return response, err
}
func TestMutation_ChangesCardinalityAndDetectsCountLeak(t *testing.T) {
	ctx := context.Background()
	conn, req := evaluationFixture(t)
	key := []byte(strings.Repeat("k", 32))
	engine, closeEngine, err := fixtureOpener(ctx, conn, key)
	require.NoError(t, err)
	defer closeEngine()
	leaking := chunkCountLeakEngine{Engine: engine, conn: conn}
	before, err := observe(ctx, leaking, req, freeCaller())
	require.NoError(t, err)
	fullBefore, err := observe(ctx, leaking, req, fullCaller())
	require.NoError(t, err)
	require.NoError(t, mutateClass(ctx, conn, freeCaller(), "body_index"))
	after, err := observe(ctx, leaking, req, freeCaller())
	require.NoError(t, err)
	fullAfter, err := observe(ctx, leaking, req, fullCaller())
	require.NoError(t, err)
	result := compareMutationClass("free-only", "body_index", before, after, fullBefore, fullAfter)
	require.False(t, result.Pass)
	require.True(t, result.Classes["body_index"].PositiveControl)
	require.Contains(t, result.Failures, InvariantFailure{Cause: build.IdentifyFailure(nil), Operation: "corpus_status", Class: "body_index", Repeat: 0})
	var removed, added, readable, postings int
	require.NoError(t, conn.QueryRow(ctx, `SELECT (SELECT count(*) FROM body_chunk WHERE id IN (1,2,4)),(SELECT count(*) FROM body_chunk WHERE id>4),(SELECT count(*) FROM body_chunk WHERE version_id=3),(SELECT count(*) FROM body_posting WHERE version_id<>3)`).Scan(&removed, &added, &readable, &postings))
	require.Zero(t, removed)
	require.Equal(t, 6, added)
	require.Equal(t, 1, readable)
	require.Equal(t, 6, postings)
	var mismatch bool
	require.NoError(t, conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM body_chunk b WHERE token_count<>(SELECT coalesce(sum(tf),0) FROM body_posting p WHERE p.chunk_id=b.id))`).Scan(&mismatch))
	require.False(t, mismatch)
	var stats int
	require.NoError(t, conn.QueryRow(ctx, `SELECT sum(chunk_count) FROM body_lexical_stats WHERE version_id<>3`).Scan(&stats))
	require.Equal(t, 6, stats)
}
