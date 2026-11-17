package evaluate

import (
	"context"
	"testing"
	"time"

	"danny.vn/trachuan/internal/frameworks/serve"
	"github.com/stretchr/testify/require"
)

func TestEvaluate_ReleaseWorkloadCounts(t *testing.T) {
	ctx := context.Background()
	req := workloadRequest()
	counts := &workloadCounts{}
	cache := NewQueryCache(workloadQuery{counts})
	require.NoError(t, cache.Prepare(ctx, req.Cases))
	copies := workloadCopies(counts, cache, req.Cases)
	engine, closeEngine, err := copies.open(ctx, nil, nil)
	require.NoError(t, err)
	defer closeEngine()
	start := time.Now()
	scoreCases(ctx, engine, req)
	status, err := engine.CorpusStatus(ctx, serve.StatusRequest{}, fullCaller())
	require.NoError(t, err)
	result := checkInvariant(ctx, req, engine, copies, nil, status)
	require.True(t, result.Pass, "failures: %+v", result.Callers)
	t.Logf("embeddings=%d opens=%d searches=%d calibration_searches=%d reads=%d mappings=%d embedding_time=%s open_time=%s total=%s", counts.embeds, counts.opens, counts.searches, counts.calibrations, counts.reads, counts.mappings, counts.embeddingTime, counts.openTime, time.Since(start))
	require.Equal(t, 60, counts.embeds)
	require.Equal(t, 16, counts.opens)
	require.Equal(t, 11445, counts.reducedSearches)
	require.Equal(t, 864, counts.calibrations)
	require.Equal(t, 11802, counts.searches)
	require.Equal(t, 3312, counts.reads)
	require.Equal(t, 1656, counts.mappings)
}
