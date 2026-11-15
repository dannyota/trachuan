//go:build integration

package store_test

import (
	"context"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/migrations"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestRestoreFence_RequiresMigrationThenFences(t *testing.T) {
	ctx := context.Background()
	url := testpg.NewDatabase(t)
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	db := stdlib.OpenDB(*conn.Config())
	defer func() { _ = db.Close() }()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.UpTo(ctx, 8); err != nil {
		t.Fatal(err)
	}
	q := store.New(conn)
	account, err := q.CreateAccount(ctx, store.CreateAccountParams{GoogleSub: "restore-synthetic", Email: "owner@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	webSession(t, ctx, conn, account.ID, strings.Repeat("a", 64))
	if err = q.RestoreFence(ctx); err == nil || err.Error() != "auth schema is not migrated; run migrate first" {
		t.Fatalf("previous schema error=%v", err)
	}
	var unchanged bool
	if err = conn.QueryRow(ctx, `SELECT a.auth_epoch=0 AND s.revoked_at IS NULL
 AND (SELECT max(version_id) FROM goose_db_version)=8 AND to_regclass('framework_entitlement') IS NULL
 FROM account a JOIN web_session s ON s.account_id=a.id WHERE a.id=$1`, account.ID).Scan(&unchanged); err != nil || !unchanged {
		t.Fatalf("previous schema changed=%v error=%v", !unchanged, err)
	}
	if err = migrate.Up(ctx, url); err != nil {
		t.Fatal(err)
	}
	if _, err = q.AllowFrameworkEntitlement(ctx, store.FrameworkAllowance{Email: account.Email, Scope: "fw-alpha", By: "operator"}); err != nil {
		t.Fatal(err)
	}
	if err = q.RestoreFence(ctx); err != nil {
		t.Fatal(err)
	}
	var fenced bool
	if err = conn.QueryRow(ctx, `SELECT a.auth_epoch=1 AND s.revoked_at IS NOT NULL
 AND (SELECT count(*) FROM framework_entitlement WHERE revoked_at IS NULL)=0
 AND (SELECT count(*) FROM framework_entitlement_event WHERE reason='restore')=1
 FROM account a JOIN web_session s ON s.account_id=a.id WHERE a.id=$1`, account.ID).Scan(&fenced); err != nil || !fenced {
		t.Fatalf("migrated schema fenced=%v error=%v", fenced, err)
	}
}
