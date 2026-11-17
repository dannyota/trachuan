//go:build integration

package build

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	schema "danny.vn/trachuan/internal/corpusdb/frameworks"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestTemporaryDatabase_DropsAfterSuccessAndFailure(t *testing.T) {
	admin, cfg, name := offlineFixture(t)
	ctx := context.Background()
	require.NoError(t, writeMarker(ctx, admin, name, "owned"))
	for _, failure := range []bool{false, true} {
		err := WithTemporaryDatabase(ctx, cfg, name, "owned", func(raw string) bool { return raw == "owned" }, func(context.Context, *pgx.Conn) error {
			if failure {
				return errors.New("invented failure")
			}
			return nil
		})
		if failure {
			require.ErrorContains(t, err, "invented failure")
		} else {
			require.NoError(t, err)
		}
		var exists bool
		require.NoError(t, admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)`, name).Scan(&exists))
		require.False(t, exists)
	}
}
func TestOfflineRestore_FrameworkFunctionsAndRecords(t *testing.T) {
	admin, cfg, name := offlineFixture(t)
	ctx := context.Background()
	cfg.PgRestore = os.Getenv("CORPUS_PG_RESTORE")
	if cfg.PgRestore == "" {
		cfg.PgRestore = "pg_restore"
	}
	require.NoError(t, writeMarker(ctx, admin, name, "owned"))
	path := filepath.Join(t.TempDir(), "invented.dump")
	owns := func(raw string) bool { return raw == "owned" }
	require.NoError(t, WithDatabase(ctx, cfg, name, "owned", owns, func(ctx context.Context, conn *pgx.Conn) error {
		require.NoError(t, schema.Apply(ctx, conn, 1024))
		_, err := conn.Exec(ctx, `CREATE TABLE invented(value integer);INSERT INTO invented VALUES(7)`)
		require.NoError(t, err)
		return DumpDatabase(ctx, cfg, name, path)
	}))
	require.NoError(t, WithDatabase(ctx, cfg, name, "owned", owns, func(ctx context.Context, conn *pgx.Conn) error {
		require.NoError(t, RestoreDatabase(ctx, cfg, name, path))
		var value int
		require.NoError(t, conn.QueryRow(ctx, `SELECT value FROM invented`).Scan(&value))
		require.Equal(t, 7, value)
		var functions int
		require.NoError(t, conn.QueryRow(ctx, `SELECT count(*) FROM pg_proc WHERE pronamespace='public'::regnamespace AND proname IN ('reject_version_mutation','check_evidence_dependencies')`).Scan(&functions))
		require.Equal(t, 2, functions)
		return nil
	}))
}

func TestTemporaryDatabase_RefusesChangedMarker(t *testing.T) {
	admin, cfg, name := offlineFixture(t)
	ctx := context.Background()
	require.NoError(t, writeMarker(ctx, admin, name, "owned"))
	err := WithTemporaryDatabase(ctx, cfg, name, "owned", func(raw string) bool { return raw == "owned" }, func(context.Context, *pgx.Conn) error { return writeMarker(ctx, admin, name, "foreign") })
	require.ErrorContains(t, err, "cleanup ownership mismatch")
	var exists bool
	require.NoError(t, admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)`, name).Scan(&exists))
	require.True(t, exists)
}
