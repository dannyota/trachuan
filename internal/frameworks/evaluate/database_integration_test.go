//go:build integration

package evaluate

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/frameworks/build"
	"danny.vn/trachuan/internal/frameworks/serve"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestMutation_RealEngineProjectionAndPositiveControls(t *testing.T) {
	ctx := context.Background()
	conn, req := evaluationFixture(t)
	require.NoError(t, verifyRestored(ctx, conn, req))
	key := []byte(strings.Repeat("k", 32))
	before, closeBefore, err := fixtureOpener(ctx, conn, key)
	require.NoError(t, err)
	a, err := observe(ctx, before, req, freeCaller())
	require.NoError(t, err)
	full, err := observe(ctx, before, req, fullCaller())
	require.NoError(t, err)
	calibration, err := build.CalibrateCases(ctx, before, req.Cases, req.CaseFileSHA256)
	require.NoError(t, err)
	closeBefore()
	for _, class := range privateClasses() {
		require.NoError(t, mutateClass(ctx, conn, freeCaller(), class))
	}
	after, closeAfter, err := fixtureOpener(ctx, conn, key)
	require.NoError(t, err)
	defer closeAfter()
	b, err := observe(ctx, after, req, freeCaller())
	require.NoError(t, err)
	fullAfter, err := observe(ctx, after, req, fullCaller())
	require.NoError(t, err)
	result := compareInvariant("free-only", a, b, full, fullAfter)
	require.True(t, result.Pass, "invariant failed: %+v", result.Failures)
	recalibration, err := build.CalibrateCases(ctx, after, req.Cases, req.CaseFileSHA256)
	require.NoError(t, err)
	require.Equal(t, calibration.Threshold, recalibration.Threshold)
	var evidence, gaps int
	require.NoError(t, conn.QueryRow(ctx, `SELECT (SELECT count(*) FROM mapping_evidence),(SELECT count(*) FROM quality_gap WHERE class='text')`).Scan(&evidence, &gaps))
	require.Equal(t, 3, evidence)
	require.Equal(t, 3, gaps)
	var public string
	require.NoError(t, conn.QueryRow(ctx, `SELECT content FROM public_chunk LIMIT 1`).Scan(&public))
	require.Equal(t, "public", public)
}

func TestInvariant_MixedCallersUseFrameworkCodes(t *testing.T) {
	ctx := context.Background()
	conn, req := evaluationFixture(t)
	key := []byte(strings.Repeat("k", 32))
	base, closeEngine, err := fixtureOpener(ctx, conn, key)
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
	result := checkInvariant(ctx, req, base, invariantCopies{restore: restore, open: fixtureOpener, mutate: mutateClass}, key, status)
	require.True(t, result.Pass, "invariant failed: %+v", result.Callers)
	require.Equal(t, "evaluated", result.MixedCoverage)
	require.Equal(t, 3, clones)
	require.Equal(t, []string{"free-only", "fw-alpha", "fw-gamma"}, []string{result.Callers[0].Caller, result.Callers[1].Caller, result.Callers[2].Caller})
	before, err := observe(ctx, base, req, freeCaller())
	require.NoError(t, err)
	for _, o := range before {
		require.False(t, bytes.Contains(o.Value, []byte("Invented publisher title fw-alpha")))
		require.False(t, bytes.Contains(o.Value, []byte("Invented publisher title fw-gamma")))
	}
}

func TestInvariant_AttributesRealEngineLeakStages(t *testing.T) {
	ctx := context.Background()
	conn, req := evaluationFixture(t)
	key := []byte(strings.Repeat("k", 32))
	opens := func(ctx context.Context, conn *pgx.Conn, key []byte) (Engine, func(), error) {
		engine, closeEngine, err := fixtureOpener(ctx, conn, key)
		if err != nil {
			return nil, nil, err
		}
		return chunkCountLeakEngine{Engine: engine, conn: conn}, closeEngine, nil
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
		require.False(t, caller.Classes["body_index"].Pass)
		require.True(t, caller.Classes["body_index"].PositiveControl)
		require.NotEmpty(t, caller.Failures)
		for _, failure := range caller.Failures {
			require.Contains(t, []string{"body_index", "mapping_evidence", "text_gaps", "source_files"}, failure.Class)
			require.Equal(t, "corpus_status", failure.Operation)
			require.NotEmpty(t, failure.Cause.GoType)
		}
	}
}
