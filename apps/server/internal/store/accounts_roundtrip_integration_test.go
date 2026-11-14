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

func accountsMigration(t *testing.T) (context.Context, *pgx.Conn, *goose.Provider) {
	t.Helper()
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
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })
	return ctx, conn, provider
}

func TestAccounts_MigrateRejectsInvalidAndDuplicates(t *testing.T) {
	for _, tc := range []struct{ name, first, second string }{
		{"invalid", "\"a\"@example.test", ""},
		{"domain collision", "Owner@EXAMPLE.TEST", "Owner@example.test"},
		{"exact collision", "owner@example.test", "owner@example.test"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, conn, provider := accountsMigration(t)
			if _, err := conn.Exec(ctx, `INSERT INTO account(google_sub,email) VALUES ('first',$1)`, tc.first); err != nil {
				t.Fatal(err)
			}
			if tc.second != "" {
				if _, err := conn.Exec(ctx, `INSERT INTO account(google_sub,email) VALUES ('second',$1)`, tc.second); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := provider.UpTo(ctx, 5); err == nil {
				t.Fatal("invalid migration succeeded")
			}
			var email string
			if err := conn.QueryRow(ctx, `SELECT email FROM account WHERE google_sub='first'`).Scan(&email); err != nil || email != tc.first {
				t.Fatal("failed migration changed account")
			}
		})
	}
}

func TestAccounts_MigrationRoundTrip(t *testing.T) {
	ctx, conn, provider := accountsMigration(t)
	if _, err := conn.Exec(ctx, `INSERT INTO account(google_sub,email) VALUES ('first',E' \tOwner@EXAMPLE.TEST\n'),('second','owner@example.test')`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO web_session(token_hash,account_id,csrf_secret,auth_epoch,expires_at) SELECT repeat('a',64),id,'synthetic',0,now()+interval '1 day' FROM account WHERE google_sub='first'`); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 5); err != nil {
		t.Fatal(err)
	}
	var normalized string
	var noProof bool
	if err := conn.QueryRow(ctx, `SELECT email FROM account WHERE google_sub='first'`).Scan(&normalized); err != nil || normalized != "Owner@example.test" {
		t.Fatalf("normalization=%q err=%v", normalized, err)
	}
	if err := conn.QueryRow(ctx, `SELECT proof_at IS NULL AND factor_at IS NULL FROM web_session`).Scan(&noProof); err != nil || !noProof {
		t.Fatal("existing session acquired proof")
	}
	if _, err := conn.Exec(ctx, `INSERT INTO password_credential(account_id,hash,version) SELECT id,'synthetic',1 FROM account WHERE google_sub='first'`); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 4); err != nil {
		t.Fatal(err)
	}
	var googleRequired bool
	if err := conn.QueryRow(ctx, `SELECT attnotnull FROM pg_attribute WHERE attrelid='account'::regclass AND attname='google_sub'`).Scan(&googleRequired); err != nil || !googleRequired {
		t.Fatal("Down did not restore Google requirement")
	}
	if _, err := provider.UpTo(ctx, 5); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 5); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM account`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("accounts=%d err=%v", count, err)
	}
}

func TestAccounts_DownRefusesPasswordOnlyDataLoss(t *testing.T) {
	ctx, conn, provider := accountsMigration(t)
	if _, err := provider.UpTo(ctx, 5); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `BEGIN;
 INSERT INTO account(email) VALUES ('password@example.test');
 INSERT INTO password_credential(account_id,hash,version) SELECT id,'synthetic',1 FROM account;
 COMMIT;`); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 4); err == nil {
		t.Fatal("Down discarded a password-only identity")
	}
	var count int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM password_credential`).Scan(&count); err != nil || count != 1 {
		t.Fatal("failed Down lost credential")
	}
}
