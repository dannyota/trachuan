//go:build integration

package build

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func offlineFixture(t *testing.T) (*pgx.Conn, Config, string) {
	t.Helper()
	ctx := context.Background()
	builder := testpg.NewBuilder(t)
	admin, _, err := testpg.OpenAdministrator(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, admin.Close(context.Background())) })
	target, err := pgx.ParseConfig(testpg.NewDatabase(t))
	require.NoError(t, err)
	name := target.Database
	_, err = admin.Exec(ctx, `ALTER DATABASE `+pgx.Identifier{name}.Sanitize()+` OWNER TO `+pgx.Identifier{builder.Role}.Sanitize())
	require.NoError(t, err)
	dump := os.Getenv("CORPUS_PG_DUMP")
	if dump == "" {
		dump = "pg_dump"
	}
	return admin, Config{BuilderURL: builder.URL, Template: builder.Template, PgDump: dump}, name
}

func TestWithDatabase_MarkerGuardAndRecreate(t *testing.T) {
	admin, cfg, name := offlineFixture(t)
	ctx := context.Background()
	called := false
	use := func(context.Context, *pgx.Conn) error { called = true; return nil }
	owns := func(raw string) bool { return raw == "owned-old" || raw == "owned-new" }
	for _, marker := range []string{"", "foreign"} {
		if marker != "" {
			require.NoError(t, writeMarker(ctx, admin, name, marker))
		}
		err := WithDatabase(ctx, cfg, name, "owned-new", owns, use)
		require.ErrorContains(t, err, "ownership mismatch")
		require.False(t, called)
	}
	require.NoError(t, writeMarker(ctx, admin, name, "owned-old"))
	var oldOID uint32
	require.NoError(t, admin.QueryRow(ctx, `SELECT oid FROM pg_database WHERE datname=$1`, name).Scan(&oldOID))
	require.NoError(t, WithDatabase(ctx, cfg, name, "owned-new", owns, func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, `CREATE TABLE invented(value integer); INSERT INTO invented VALUES(1)`)
		return err
	}))
	marker, err := readMarker(ctx, admin, name)
	require.NoError(t, err)
	require.Equal(t, "owned-new", marker)
	var newOID uint32
	require.NoError(t, admin.QueryRow(ctx, `SELECT oid FROM pg_database WHERE datname=$1`, name).Scan(&newOID))
	require.NotEqual(t, oldOID, newOID)
	require.NoError(t, WithDatabase(ctx, cfg, name, "owned-new", owns, func(ctx context.Context, conn *pgx.Conn) error {
		var absent bool
		err := conn.QueryRow(ctx, `SELECT to_regclass('invented') IS NULL`).Scan(&absent)
		require.True(t, absent)
		return err
	}))
}

func TestWithDatabase_LockCoversCallbackAndCancellation(t *testing.T) {
	admin, cfg, name := offlineFixture(t)
	ctx := context.Background()
	require.NoError(t, writeMarker(ctx, admin, name, "owned"))
	owns := func(raw string) bool { return raw == "owned" }
	require.NoError(t, WithDatabase(ctx, cfg, name, "owned", owns, func(ctx context.Context, _ *pgx.Conn) error {
		other, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		called := false
		err := WithDatabase(other, cfg, name, "owned", owns, func(context.Context, *pgx.Conn) error { called = true; return nil })
		require.Error(t, err)
		require.False(t, called)
		return nil
	}))
	ctx, cancel := context.WithCancel(ctx)
	err := WithDatabase(ctx, cfg, name, "owned", owns, func(context.Context, *pgx.Conn) error { cancel(); return ctx.Err() })
	require.ErrorIs(t, err, context.Canceled)
	var exists bool
	require.NoError(t, admin.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)`, name).Scan(&exists))
	require.True(t, exists)
}

func TestWithDatabase_DumpOmitsOwnershipMarker(t *testing.T) {
	admin, cfg, name := offlineFixture(t)
	ctx := context.Background()
	marker := "synthetic-private-work-directory-marker"
	require.NoError(t, writeMarker(ctx, admin, name, marker))
	dump := filepath.Join(t.TempDir(), "frameworks.dump")
	require.NoError(t, WithDatabase(ctx, cfg, name, marker, func(raw string) bool { return raw == marker }, func(ctx context.Context, conn *pgx.Conn) error {
		if _, err := conn.Exec(ctx, `CREATE TABLE invented(value text); INSERT INTO invented VALUES('invented text')`); err != nil {
			return err
		}
		return DumpDatabase(ctx, cfg, name, dump)
	}))
	restore := os.Getenv("CORPUS_PG_RESTORE")
	if restore == "" {
		restore = "pg_restore"
	}
	listing, err := exec.CommandContext(ctx, restore, "--list", dump).Output()
	require.NoError(t, err)
	require.NotContains(t, string(listing), "COMMENT")
	require.NotContains(t, string(listing), marker)
	err = WithDatabase(ctx, cfg, name, marker, func(raw string) bool { return raw == marker }, func(context.Context, *pgx.Conn) error { return errors.New("invented failure") })
	require.ErrorContains(t, err, "invented failure")
}
