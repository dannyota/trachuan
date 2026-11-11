//go:build integration

package migrate_test

import (
	"context"
	"testing"

	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
)

func TestMigrate_UpTwiceIsNoop(t *testing.T) {
	ctx := context.Background()
	url := testpg.NewDatabase(t)
	for i := 0; i < 2; i++ {
		if err := migrate.Up(ctx, url); err != nil {
			t.Fatalf("apply migrations: %v", err)
		}
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	var count int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM goose_db_version WHERE version_id = 1 AND is_applied`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("migration applied %d times, want 1", count)
	}
}
