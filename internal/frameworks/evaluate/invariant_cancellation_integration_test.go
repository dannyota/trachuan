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

type cancellingCountLeakEngine struct {
	Engine
	conn *pgx.Conn
}

func (e cancellingCountLeakEngine) CorpusStatus(ctx context.Context, r serve.StatusRequest, caller serve.Caller) (serve.StatusResponse, error) {
	response, err := e.Engine.CorpusStatus(ctx, r, caller)
	if err != nil {
		return response, err
	}
	var count int
	err = e.conn.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM body_chunk b JOIN framework_version v ON v.id=b.version_id WHERE v.access='licensed')-
 (SELECT count(*) FROM quality_gap g JOIN framework_version v ON v.id=g.version_id WHERE v.access='licensed' AND g.class='text')`).Scan(&count)
	response.Totals.Controls += count
	return response, err
}
func TestInvariant_DetectsCancellingPrivateLeaks(t *testing.T) {
	ctx := context.Background()
	conn, req := evaluationFixture(t)
	key := []byte(strings.Repeat("k", 32))
	opens := func(ctx context.Context, conn *pgx.Conn, key []byte) (Engine, func(), error) {
		engine, closeEngine, err := fixtureOpener(ctx, conn, key)
		if err != nil {
			return nil, nil, err
		}
		return cancellingCountLeakEngine{Engine: engine, conn: conn}, closeEngine, nil
	}
	base, closeEngine, err := opens(ctx, conn, key)
	require.NoError(t, err)
	defer closeEngine()
	status, err := base.CorpusStatus(ctx, serve.StatusRequest{}, fullCaller())
	require.NoError(t, err)
	clones := 0
	restore := func(ctx context.Context, _ build.DatabaseRequest, _ string, temporary bool, use func(context.Context, *pgx.Conn) error) error {
		require.True(t, temporary)
		clones++
		copyConn, _ := evaluationFixture(t)
		return use(ctx, copyConn)
	}
	result := checkInvariant(ctx, req, base, invariantCopies{restore: restore, open: opens, mutate: mutateClass}, key, status)
	require.False(t, result.Pass)
	require.Equal(t, 3, clones)
	for _, caller := range result.Callers {
		require.False(t, caller.Pass)
		require.False(t, caller.Classes["body_index"].Pass)
		require.False(t, caller.Classes["mapping_evidence"].Pass)
		for _, class := range []string{"control_text", "text_gaps", "source_files"} {
			require.True(t, caller.Classes[class].Pass, class)
		}
		for _, class := range privateClasses() {
			require.True(t, caller.Classes[class].PositiveControl, class)
		}
		require.Len(t, caller.Failures, 4)
		for _, failure := range caller.Failures {
			require.Contains(t, []string{"body_index", "mapping_evidence"}, failure.Class)
			require.Equal(t, "corpus_status", failure.Operation)
			require.NotEmpty(t, failure.Cause.GoType)
		}
	}
}
