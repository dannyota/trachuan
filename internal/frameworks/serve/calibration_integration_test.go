//go:build integration

package serve

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/frameworks/build"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func fixtureGoldenCases(t *testing.T, root string) {
	t.Helper()
	csv := "id,question,framework_code,version_label,citation_norm,expect_abstain,notes,adjudication,reason\n" +
		"positive,quartz safeguard,fw-beta,1,A-1,false,,accepted,invented review\n" +
		"negative,nebula unrelated,absent,,,true,,accepted,invented review\n" +
		"lookup,fw-beta A-1,fw-beta,1,A-1,false,,accepted,invented review\n" +
		"licensed,never fit licensed,fw-alpha,2,A-1,false,,accepted,invented review\n" +
		"unverified,never fit unverified,fw-beta,1,A-1,false,,unverified,invented review\n" +
		"outside,never fit outside,fw-beta,99,A-1,false,,accepted,invented review\n"
	dir := filepath.Join(root, "source/trachuan")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "golden.csv"), []byte(csv), 0600))
	review := fmt.Sprintf("reviews:\n  - release: '20990101'\n    reviewer: coordinator\n    date: '2099-01-01'\n    csv_sha256: %x\n    added: [positive, negative, lookup, licensed, unverified, outside]\n    rejected: []\n    retargeted: []\n    note: invented fixture\n", sha256.Sum256([]byte(csv)))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "golden-review.yaml"), []byte(review), 0600))
}

func openFixtureCalibration(ctx context.Context, conn *pgx.Conn) (build.CalibrationEngine, func(), error) {
	pool, err := pgxpool.New(ctx, conn.Config().ConnString())
	if err != nil {
		return nil, nil, err
	}
	engine, err := Open(ctx, pool, syntheticQuery{}, Options{CursorKey: []byte(strings.Repeat("c", 32))})
	if err != nil {
		pool.Close()
		return nil, nil, err
	}
	return engine, pool.Close, nil
}

func calibrationRecord(t *testing.T, engine *Engine) build.CalibrationRecord {
	t.Helper()
	ctx := context.Background()
	var raw []byte
	require.NoError(t, engine.db.QueryRow(ctx, `SELECT retrieval_recipe FROM release`).Scan(&raw))
	var r recipe
	require.NoError(t, decodeRecipe(raw, &r))
	require.NoError(t, r.validate())
	require.Equal(t, "measured", r.Status)
	require.Equal(t, .5, *r.Abstention.Threshold)
	raw, err := os.ReadFile(filepath.Join(os.Getenv("FRAMEWORKS_WORK_DIR"), "20990101/build/calibration.json"))
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(raw)), r.Abstention.CalibrationSHA256)
	var record build.CalibrationRecord
	require.NoError(t, json.Unmarshal(raw, &record))
	require.Len(t, record.Scores, 3)
	require.Equal(t, 1, record.LookupCount)
	require.NotContains(t, string(raw), "quartz")
	return record
}

func TestCalibration_LicensedBodyMutationDoesNotChangeMeasuredThreshold(t *testing.T) {
	a, traceA := buildFixtureWithCases(t, false, false, true)
	recordA := calibrationRecord(t, a)
	b, traceB := buildFixtureWithCases(t, true, false, true)
	recordB := calibrationRecord(t, b)
	require.Equal(t, recordA, recordB)
	ctx := context.Background()
	for _, engine := range []*Engine{a, b} {
		threshold := 2.0
		engine.recipe.Abstention.Threshold = &threshold
	}
	for _, query := range []string{"quartz safeguard", "nebula unrelated", "fw-beta A-1"} {
		traceA.reset()
		traceB.reset()
		ca, la, ea := a.CalibrationSearch(ctx, query)
		cb, lb, eb := b.CalibrationSearch(ctx, query)
		require.NoError(t, ea)
		require.NoError(t, eb)
		require.Equal(t, ca, cb)
		require.Equal(t, la, lb)
		traceA.assertProjection(t, a.project(fakeCaller()).readable)
		traceB.assertProjection(t, b.project(fakeCaller()).readable)
		if la {
			require.Nil(t, ca)
		} else {
			require.NotNil(t, ca)
			require.Less(t, *ca, 2.0)
		}
	}
	ra, ea := a.Search(ctx, Request{Query: "nebula", Limit: 50}, fakeCaller("fw-alpha"))
	rb, eb := b.Search(ctx, Request{Query: "nebula", Limit: 50}, fakeCaller("fw-alpha"))
	require.NoError(t, ea)
	require.NoError(t, eb)
	require.NotEqual(t, responseBytes(t, ra, ea), responseBytes(t, rb, eb))
}
