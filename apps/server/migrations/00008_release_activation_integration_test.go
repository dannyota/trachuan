//go:build integration

package migrations_test

import (
	"context"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/migrations"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
)

func TestActivationMigration_PreservesLegacyRowsAndReverses(t *testing.T) {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testpg.NewDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	base, err := migrations.FS.ReadFile("00001_auth.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := splitMigration(t, base)
	if _, err = conn.Exec(ctx, up[0]); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, `INSERT INTO retired_release VALUES ('vn','corpus_vn_20990101_1',now(),now()+interval '7 days',NULL);
		INSERT INTO release_journal (country,rel,step,state,message) VALUES ('vn','20990101_1','legacy','done','synthetic');`); err != nil {
		t.Fatal(err)
	}
	raw, err := migrations.FS.ReadFile("00008_release_activation.sql")
	if err != nil {
		t.Fatal(err)
	}
	parts := splitMigration(t, raw)
	for _, sql := range []string{parts[0], parts[1], parts[0]} {
		if _, err := conn.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	var digest *string
	var count int
	if err := conn.QueryRow(ctx, `SELECT dump_sha256 FROM retired_release`).Scan(&digest); err != nil || digest != nil {
		t.Fatalf("legacy digest: %v, %v", digest, err)
	}
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM release_journal WHERE operation_id IS NULL AND message='synthetic'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("legacy journal: %d, %v", count, err)
	}
}

func splitMigration(t *testing.T, raw []byte) []string {
	t.Helper()
	parts := strings.Split(string(raw), "-- +goose Down")
	if len(parts) != 2 {
		t.Fatal("missing down migration")
	}
	return parts
}
