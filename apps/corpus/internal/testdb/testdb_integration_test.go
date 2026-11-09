//go:build integration

package testdb

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestNew_MigratedAndIsolated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	first, err := pgx.Connect(ctx, New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close(ctx) }()
	if _, execErr := first.Exec(ctx, `CREATE TABLE test_isolation (id int)`); execErr != nil {
		t.Fatal(execErr)
	}
	second, err := pgx.Connect(ctx, New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close(ctx) }()
	var migrated, isolated bool
	if err := second.QueryRow(ctx, `SELECT to_regclass('run') IS NOT NULL, to_regclass('test_isolation') IS NULL`).Scan(&migrated, &isolated); err != nil {
		t.Fatal(err)
	}
	if !migrated || !isolated {
		t.Fatalf("migrated=%v isolated=%v", migrated, isolated)
	}
}

func TestNewEmpty_HasNoMigrations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := pgx.Connect(ctx, NewEmpty(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close(ctx) }()
	var empty bool
	if err := db.QueryRow(ctx, `SELECT to_regclass('run') IS NULL`).Scan(&empty); err != nil {
		t.Fatal(err)
	}
	if !empty {
		t.Fatal("empty database contains run table")
	}
}
