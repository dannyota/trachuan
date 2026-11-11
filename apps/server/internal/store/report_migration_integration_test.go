//go:build integration

package store_test

import (
	"context"
	"testing"

	"danny.vn/trachuan/apps/server/migrations"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestMigrate_00004RoundTrip(t *testing.T) {
	ctx := context.Background()
	config, err := pgx.ParseConfig(testpg.NewDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDB(*config)
	t.Cleanup(func() { _ = db.Close() })
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.UpTo(ctx, 4); err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(ctx, 3); err != nil {
		t.Fatal(err)
	}
	var reports, snapshots, account *string
	if err = db.QueryRowContext(ctx, `SELECT to_regclass('issue_report')::text, to_regclass('issue_report_export')::text, to_regclass('account')::text`).Scan(&reports, &snapshots, &account); err != nil {
		t.Fatal(err)
	}
	if reports != nil || snapshots != nil || account == nil {
		t.Fatalf("down: reports=%v snapshots=%v account=%v", reports, snapshots, account)
	}
	if _, err = provider.UpTo(ctx, 4); err != nil {
		t.Fatal(err)
	}
}
