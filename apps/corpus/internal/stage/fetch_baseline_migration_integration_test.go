//go:build integration

package stage

import (
	"context"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"danny.vn/trachuan/apps/corpus/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestFetchBaselineMigration_UpAndDown(t *testing.T) {
	ctx := context.Background()
	cfg, err := pgx.ParseConfig(testdb.NewEmpty(t))
	if err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDB(*cfg)
	defer func() { _ = db.Close() }()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.UpTo(ctx, 6); err != nil {
		t.Fatal(err)
	}
	if _, err = provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	var exists bool
	if err = db.QueryRowContext(ctx, `SELECT to_regclass('fetch_baseline') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatalf("up: exists=%v: %v", exists, err)
	}
	if _, err = provider.Down(ctx); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, `SELECT to_regclass('fetch_baseline') IS NOT NULL`).Scan(&exists); err != nil || exists {
		t.Fatalf("down: exists=%v: %v", exists, err)
	}
}
