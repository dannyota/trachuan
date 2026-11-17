//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"danny.vn/trachuan/apps/server/migrations"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
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
	var serverExists bool
	if err = conn.QueryRow(ctx, `SELECT EXISTS (SELECT FROM pg_roles WHERE rolname='server')`).Scan(&serverExists); err != nil {
		t.Fatal(err)
	}
	if serverExists {
		for _, privilege := range []string{"SELECT", "INSERT", "UPDATE", "DELETE", "TRUNCATE", "REFERENCES", "TRIGGER"} {
			var allowed bool
			if err = conn.QueryRow(ctx, `SELECT has_table_privilege('server','frameworks_release',$1)`, privilege).Scan(&allowed); err != nil || allowed != (privilege == "SELECT") {
				t.Fatalf("server %s=%t: %v", privilege, allowed, err)
			}
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
