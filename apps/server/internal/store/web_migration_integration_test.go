//go:build integration

package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/apps/server/migrations"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestMigrate_00002UpTwice(t *testing.T) {
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
	t.Cleanup(func() { _ = conn.Close(ctx) })
	var count int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM goose_db_version WHERE version_id = 2 AND is_applied`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("migration 00002 applied %d times, want 1", count)
	}
}

func TestMigrate_00002PopulatedRoundTrip(t *testing.T) {
	ctx := context.Background()
	url := testpg.NewDatabase(t)
	config, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDB(*config)
	t.Cleanup(func() { _ = db.Close() })
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.UpTo(ctx, 1); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })
	var survivor, cascade int64
	if err = conn.QueryRow(ctx, `INSERT INTO account (google_sub, email) VALUES ('survivor', 'test@example.invalid') RETURNING id`).Scan(&survivor); err != nil {
		t.Fatal(err)
	}
	if err = conn.QueryRow(ctx, `INSERT INTO account (google_sub, email) VALUES ('cascade', 'test@example.invalid') RETURNING id`).Scan(&cascade); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		account int64
		hash    string
	}{{survivor, "a"}, {cascade, "b"}} {
		if _, err = conn.Exec(ctx, `INSERT INTO personal_token (token_hash, account_id, name, expires_at, auth_epoch) VALUES ($1, $2, 'test', now() + interval '1 day', 0)`, strings.Repeat(tc.hash, 64), tc.account); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err = conn.QueryRow(ctx, `SELECT id FROM personal_token WHERE token_hash = $1`, strings.Repeat("a", 64)).Scan(&id); err != nil || id == 0 {
		t.Fatalf("upgraded token id=%d err=%v", id, err)
	}
	if _, err = conn.Exec(ctx, `DELETE FROM account WHERE id = $1`, cascade); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = conn.QueryRow(ctx, `SELECT count(*) FROM personal_token WHERE account_id = $1`, cascade).Scan(&count); err != nil || count != 0 {
		t.Fatalf("cascade tokens=%d err=%v", count, err)
	}
	if _, err = provider.Down(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec(ctx, `DELETE FROM account WHERE id = $1`, survivor)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		t.Fatalf("delete after down: %v, want foreign key violation", err)
	}
	if _, err = provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if err = conn.QueryRow(ctx, `SELECT id FROM personal_token WHERE token_hash = $1 AND account_id = $2`, strings.Repeat("a", 64), survivor).Scan(&id); err != nil || id == 0 {
		t.Fatalf("round-trip token id=%d err=%v", id, err)
	}
}
