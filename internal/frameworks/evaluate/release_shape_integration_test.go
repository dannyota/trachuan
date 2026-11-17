//go:build integration

package evaluate

import (
	"context"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/frameworks/build"
	"danny.vn/trachuan/internal/frameworks/serve"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func releaseShapedFixture(t *testing.T) (*pgx.Conn, EvaluationRequest) {
	t.Helper()
	conn, req := evaluationFixture(t)
	ctx := context.Background()
	queries := []string{
		`INSERT INTO control(id,version_id,ordinal,citation,citation_norm,kind,title,status) SELECT n,1+n%4,2+n/4,'X-'||n,'X-'||n,'control','Invented public title','active' FROM generate_series(100,4038) n`,
		`INSERT INTO control_text(control_id,title_original,body,notes,tables) SELECT id,'Invented original title '||version_id,'Invented security accountability procedure','{}','{}' FROM control WHERE id>=100`,
		`INSERT INTO body_chunk(id,version_id,control_id,ordinal,content,input_sha256,token_count,embedding) SELECT c.id,c.version_id,c.id,0,(SELECT string_agg('invented'||n,' ' ORDER BY n) FROM generate_series(1,128) n),b.input_sha256,128,b.embedding FROM control c JOIN body_chunk b ON b.id=c.version_id WHERE c.id>=100`,
		`INSERT INTO body_posting(version_id,chunk_id,token,tf) SELECT b.version_id,b.id,'invented'||n,1 FROM body_chunk b CROSS JOIN generate_series(1,128) n WHERE b.id>=100`,
		`INSERT INTO public_chunk(id,version_id,control_id,content,input_sha256,token_count,embedding) SELECT c.id,c.version_id,c.id,'invented public metadata',b.input_sha256,3,b.embedding FROM control c JOIN body_chunk b ON b.id=c.version_id WHERE c.id>=100`,
		`INSERT INTO public_posting(version_id,chunk_id,token,tf) SELECT b.version_id,b.id,token,1 FROM public_chunk b CROSS JOIN unnest(ARRAY['invented','public','metadata']) token WHERE b.id>=100`,
		`INSERT INTO control_mapping(id,from_code,from_version,from_citation,to_code,to_version,to_citation,relation,source,from_version_id,to_version_id,resolved)
 SELECT c.id,v.framework_code,v.version,c.citation,'fw-beta','1','X-'||(c.id/4*4+2),'related','invented',c.version_id,3,true
 FROM control c JOIN framework_version v ON v.id=c.version_id WHERE c.id>=100 AND c.version_id<>3`,
		`UPDATE release SET control_count=(SELECT count(*) FROM control)`,
	}
	for _, query := range queries {
		_, err := conn.Exec(ctx, query)
		require.NoError(t, err)
	}
	return conn, req
}

func TestInvariant_ReleaseShapedFirstStage(t *testing.T) {
	ctx := context.Background()
	conn, req := releaseShapedFixture(t)
	require.NoError(t, verifyRestored(ctx, conn, req))
	key := []byte(strings.Repeat("k", 32))
	var before []invariantObservation
	for _, mutated := range []bool{false, true} {
		if mutated {
			require.NoError(t, mutateClass(ctx, conn, freeCaller(), "control_text"))
		}
		engine, closeEngine, err := fixtureOpener(ctx, conn, key)
		require.NoError(t, err)
		started := time.Now()
		observations, err := observe(ctx, engine, req, freeCaller())
		t.Logf("mutated=%t elapsed=%s cause=%+v", mutated, time.Since(started), build.IdentifyFailure(err))
		require.NoError(t, err, "safe cause: %+v", build.IdentifyFailure(err))
		if mutated {
			require.Equal(t, before, observations)
		} else {
			before = observations
		}
		_, err = engine.Search(ctx, serve.Request{Query: "absent", Limit: 8}, fullCaller())
		require.NoError(t, err, "safe cause: %+v", build.IdentifyFailure(err))
		closeEngine()
	}
}

type deadlineQueryFixture struct {
	queryFixture
	calls int
}

func (q *deadlineQueryFixture) Embed(ctx context.Context, text string) (embedding.Vector, error) {
	q.calls++
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < 3*time.Second {
		return embedding.Vector{}, context.DeadlineExceeded
	}
	return q.queryFixture.Embed(ctx, text)
}
func TestInvariant_ColdQueryOutsideSearchDeadline(t *testing.T) {
	ctx := context.Background()
	conn, req := releaseShapedFixture(t)
	require.NoError(t, mutateClass(ctx, conn, freeCaller(), "control_text"))
	key := []byte(strings.Repeat("k", 32))
	cold := &deadlineQueryFixture{}
	engine, closeEngine, err := fixtureQueryOpener(ctx, conn, key, NewQueryCache(cold))
	require.NoError(t, err)
	_, _, err = observeSearch(ctx, engine, caseRequest(req.Cases[1], "open"), freeCaller(), req.Cases[1], "open", 0)
	require.Error(t, err)
	failure := failedInvariant("free-only", "observe_mutated", err)
	require.Equal(t, "context.deadlineExceededError", failure.Failures[0].Cause.GoType)
	closeEngine()
	cache := NewQueryCache(cold)
	require.NoError(t, cache.Prepare(ctx, req.Cases))
	calls := cold.calls
	engine, closeEngine, err = fixtureQueryOpener(ctx, conn, key, cache)
	require.NoError(t, err)
	defer closeEngine()
	_, _, err = observeSearch(ctx, engine, caseRequest(req.Cases[1], "open"), freeCaller(), req.Cases[1], "open", 0)
	require.NoError(t, err)
	_, _, err = engine.CalibrationSearch(ctx, req.Cases[1].Question)
	require.NoError(t, err)
	require.Equal(t, calls, cold.calls)
}
