//go:build integration

package store_test

import (
	"context"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/apps/server/migrations"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestMigrate_00003UpTwice(t *testing.T) {
	ctx := context.Background()
	url := testpg.NewDatabase(t)
	for range 2 {
		if err := migrate.Up(ctx, url); err != nil {
			t.Fatal(err)
		}
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })
	var count int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM goose_db_version WHERE version_id = 3 AND is_applied`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("migration 00003 applied %d times, want 1", count)
	}
}

func TestMigrate_00003PopulatedRoundTrip(t *testing.T) {
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
	if _, err = provider.UpTo(ctx, 2); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })
	var account int64
	if err = conn.QueryRow(ctx, `INSERT INTO account (google_sub, email) VALUES ('oauth-roundtrip', 'test@example.invalid') RETURNING id`).Scan(&account); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, `INSERT INTO web_session (token_hash, account_id, csrf_secret, auth_epoch, expires_at) VALUES ($1, $2, 'secret', 0, now() + interval '1 day')`, strings.Repeat("a", 64), account); err != nil {
		t.Fatal(err)
	}
	if _, err = provider.UpTo(ctx, 3); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, `INSERT INTO oauth_client (client_id, kind, name, redirect_uris) VALUES ('client', 'dcr', 'Client', ARRAY['https://example.invalid/cb'])`); err != nil {
		t.Fatal(err)
	}
	var grant int64
	if err = conn.QueryRow(ctx, `INSERT INTO oauth_grant (account_id, client_id, scope, resource) VALUES ($1, 'client', 'mcp:read', 'https://example.invalid/mcp') RETURNING id`, account).Scan(&grant); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, `INSERT INTO oauth_code (code_hash, grant_id, account_id, client_id, redirect_uri, resource, code_challenge, scope, auth_epoch, expires_at) VALUES ($1, $2, $3, 'client', 'https://example.invalid/cb', 'https://example.invalid/mcp', 'challenge', 'mcp:read', 0, now() + interval '1 minute')`, strings.Repeat("b", 64), grant, account); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, `INSERT INTO oauth_request (handle_hash, browser_hash, client_id, redirect_uri, state, code_challenge, resource, scope, expires_at) VALUES ($1, $2, 'client', 'https://example.invalid/cb', 'state', 'challenge', 'https://example.invalid/mcp', 'mcp:read', now() + interval '1 minute')`, strings.Repeat("c", 64), strings.Repeat("d", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, `INSERT INTO oauth_token (token_hash, kind, grant_id, account_id, client_id, family_id, resource, scope, auth_epoch, expires_at, family_expires_at) VALUES ($1, 'refresh', $2, $3, 'client', $1, 'https://example.invalid/mcp', 'mcp:read', 0, now() + interval '1 hour', now() + interval '1 hour')`, strings.Repeat("e", 64), grant, account); err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(ctx, 2); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = conn.QueryRow(ctx, `SELECT count(*) FROM web_session WHERE account_id = $1`, account).Scan(&count); err != nil || count != 1 {
		t.Fatalf("surviving sessions=%d err=%v", count, err)
	}
	if _, err = provider.UpTo(ctx, 3); err != nil {
		t.Fatal(err)
	}
	if err = conn.QueryRow(ctx, `SELECT count(*) FROM oauth_grant`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("grants after reapply=%d err=%v", count, err)
	}
}
