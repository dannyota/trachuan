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

type integrationWorkloadQuery struct{ counts *workloadCounts }

func (integrationWorkloadQuery) Recipe(context.Context) (embedding.Recipe, error) {
	return embedding.QueryRecipe1(), nil
}
func (q integrationWorkloadQuery) Embed(ctx context.Context, text string) (embedding.Vector, error) {
	q.counts.embeds++
	if strings.HasPrefix(text, "fixture-negative") {
		text = "absent"
	}
	return queryFixture{}.Embed(ctx, text)
}

type integrationWorkloadEngine struct {
	Engine
	counts *workloadCounts
}

func (e integrationWorkloadEngine) Search(ctx context.Context, r serve.Request, c serve.Caller) (serve.Response, error) {
	e.counts.searches++
	return e.Engine.Search(ctx, r, c)
}
func (e integrationWorkloadEngine) Read(ctx context.Context, r serve.ReadRequest, c serve.Caller) (serve.ReadResponse, error) {
	e.counts.reads++
	return e.Engine.Read(ctx, r, c)
}
func (e integrationWorkloadEngine) Mappings(ctx context.Context, r serve.MappingsRequest, c serve.Caller) (serve.MappingsResponse, error) {
	e.counts.mappings++
	return e.Engine.Mappings(ctx, r, c)
}
func (e integrationWorkloadEngine) CalibrationSearch(ctx context.Context, query string) (*float64, bool, error) {
	e.counts.calibrations++
	return e.Engine.CalibrationSearch(ctx, query)
}
func TestEvaluate_IntegrationWorkload(t *testing.T) {
	started := time.Now()
	ctx := context.Background()
	conn, req := evaluationFixture(t)
	req.Cases = workloadRequest().Cases
	for i := range req.Cases {
		if !req.Cases[i].Negative {
			req.Cases[i].Target = "fw:" + req.Cases[i].Code + ":1#A"
			req.Cases[i].Question += " needle"
		}
	}
	counts := &workloadCounts{}
	cache := NewQueryCache(integrationWorkloadQuery{counts})
	require.NoError(t, cache.Prepare(ctx, req.Cases))
	copies := invariantCopies{
		open: func(ctx context.Context, conn *pgx.Conn, key []byte) (Engine, func(), error) {
			counts.opens++
			engine, closeEngine, err := fixtureQueryOpener(ctx, conn, key, cache)
			if err != nil {
				return nil, nil, err
			}
			return integrationWorkloadEngine{Engine: engine, counts: counts}, closeEngine, nil
		},
		restore: func(ctx context.Context, _ build.DatabaseRequest, _ string, temporary bool, use func(context.Context, *pgx.Conn) error) error {
			require.True(t, temporary)
			copyConn, _ := evaluationFixture(t)
			return use(ctx, copyConn)
		},
		mutate: mutateClass,
	}
	key := []byte(strings.Repeat("k", 32))
	base, closeEngine, err := copies.open(ctx, conn, key)
	require.NoError(t, err)
	defer closeEngine()
	scoreCases(ctx, base, req)
	status, err := base.CorpusStatus(ctx, serve.StatusRequest{}, fullCaller())
	require.NoError(t, err)
	result := checkInvariant(ctx, req, base, copies, key, status)
	require.True(t, result.Pass, "invariant failed: %+v", result.Callers)
	require.Equal(t, 60, counts.embeds)
	require.Equal(t, 144, counts.calibrations)
	require.Equal(t, 16, counts.opens)
	require.Less(t, counts.searches, 1800)
	t.Logf("cases=60 searches=%d calibration_searches=%d reads=%d mappings=%d opens=%d embeddings=%d elapsed=%s", counts.searches, counts.calibrations, counts.reads, counts.mappings, counts.opens, counts.embeds, time.Since(started))
}
