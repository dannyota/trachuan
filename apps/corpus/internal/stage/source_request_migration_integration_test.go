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

func TestSourceRequestMigration_UpAndDown(t *testing.T) {
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
	if _, err = provider.UpTo(ctx, 8); err != nil {
		t.Fatal(err)
	}
	if _, err = provider.UpTo(ctx, 9); err != nil {
		t.Fatal(err)
	}
	var columns string
	if err = db.QueryRowContext(ctx, `SELECT string_agg(column_name::text, ',' ORDER BY ordinal_position) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'run_source_request'`).Scan(&columns); err != nil {
		t.Fatal(err)
	}
	want := "run_id,stage,source,endpoint,attempt,started_at,duration_ms,status,outcome"
	if columns != want {
		t.Fatalf("columns=%s", columns)
	}
	if _, err = provider.Down(ctx); err != nil {
		t.Fatal(err)
	}
	var exists bool
	if err = db.QueryRowContext(ctx, `SELECT to_regclass('run_source_request') IS NOT NULL`).Scan(&exists); err != nil || exists {
		t.Fatalf("down: exists=%v err=%v", exists, err)
	}
}
