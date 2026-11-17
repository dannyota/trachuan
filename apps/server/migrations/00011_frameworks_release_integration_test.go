//go:build integration

package migrations_test

import (
	"context"
	"errors"
	"testing"

	"danny.vn/trachuan/apps/server/migrations"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestFrameworksReleaseMigration_SeparateReadOnlyPointer(t *testing.T) {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testpg.NewDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	for _, file := range []string{"00001_auth.sql", "00011_frameworks_release.sql"} {
		raw, readErr := migrations.FS.ReadFile(file)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err = conn.Exec(ctx, splitMigration(t, raw)[0]); err != nil {
			t.Fatal(err)
		}
	}
	for _, query := range []string{
		`INSERT INTO corpus_release VALUES ('vn','corpus_vn_20990101',now(),'invented')`,
		`INSERT INTO frameworks_release VALUES ('vn','frameworks_20990101',now(),'invented')`,
		`UPDATE frameworks_release SET active_db='frameworks_20990102' WHERE country='vn'`,
	} {
		if _, err = conn.Exec(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	var law string
	if err = conn.QueryRow(ctx, `SELECT active_db FROM corpus_release WHERE country='vn'`).Scan(&law); err != nil || law != "corpus_vn_20990101" {
		t.Fatalf("frameworks activation changed law: %v", err)
	}
	if _, err = conn.Exec(ctx, `INSERT INTO frameworks_release VALUES ('vn','frameworks_20990103',now(),'invented')`); err == nil {
		t.Fatal("duplicate country accepted")
	}
	for _, column := range []string{"country", "active_db", "activated_at", "activated_by"} {
		var nullable string
		if err = conn.QueryRow(ctx, `SELECT is_nullable FROM information_schema.columns WHERE table_name='frameworks_release' AND column_name=$1`, column).Scan(&nullable); err != nil || nullable != "NO" {
			t.Fatalf("nullable %s: %v", column, err)
		}
	}
	raw, err := migrations.FS.ReadFile("00011_frameworks_release.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, splitMigration(t, raw)[1]); err != nil {
		t.Fatal(err)
	}
}

func TestFrameworksReleaseMigration_ServerCanSelectButCannotWrite(t *testing.T) {
	ctx := t.Context()
	conn, err := pgx.Connect(ctx, testpg.NewDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	_, err = tx.Exec(ctx, `DO $$ BEGIN IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='server') THEN CREATE ROLE server NOLOGIN; END IF; END $$`)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := migrations.FS.ReadFile("00011_frameworks_release.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, splitMigration(t, raw)[0]); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO frameworks_release VALUES('vn','frameworks_20990101',now(),'invented')`); err != nil {
		t.Fatal(err)
	}
	for _, privilege := range []string{"SELECT", "INSERT", "UPDATE", "DELETE", "TRUNCATE", "REFERENCES", "TRIGGER"} {
		var allowed bool
		if err = tx.QueryRow(ctx, `SELECT has_table_privilege('server','frameworks_release',$1)`, privilege).Scan(&allowed); err != nil || allowed != (privilege == "SELECT") {
			t.Fatalf("server %s=%t: %v", privilege, allowed, err)
		}
	}
	if _, err = tx.Exec(ctx, `SET LOCAL ROLE server`); err != nil {
		t.Fatal(err)
	}
	var active string
	if err = tx.QueryRow(ctx, `SELECT active_db FROM frameworks_release WHERE country='vn'`).Scan(&active); err != nil || active != "frameworks_20990101" {
		t.Fatalf("server SELECT: active=%q error=%v", active, err)
	}
	for _, statement := range []string{
		`INSERT INTO frameworks_release VALUES('us','frameworks_20990102',now(),'invented')`,
		`UPDATE frameworks_release SET active_db='frameworks_20990102' WHERE country='vn'`,
		`DELETE FROM frameworks_release WHERE country='vn'`,
	} {
		if _, err = tx.Exec(ctx, `SAVEPOINT denied_write`); err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, statement)
		var denied *pgconn.PgError
		if !errors.As(err, &denied) || denied.Code != "42501" {
			t.Fatalf("server write was not denied: %s: %v", statement, err)
		}
		if _, err = tx.Exec(ctx, `ROLLBACK TO SAVEPOINT denied_write`); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, `RELEASE SAVEPOINT denied_write`); err != nil {
			t.Fatal(err)
		}
	}
}
