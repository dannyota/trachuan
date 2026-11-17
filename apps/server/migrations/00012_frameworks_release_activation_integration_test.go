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

func TestFrameworksActivationMigration_LegacyUpdaterAndKindChecks(t *testing.T) {
	ctx := t.Context()
	conn, err := pgx.Connect(ctx, testpg.NewDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	for _, name := range []string{"00001_auth.sql", "00008_release_activation.sql", "00011_frameworks_release.sql"} {
		raw, readErr := migrations.FS.ReadFile(name)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err = conn.Exec(ctx, splitMigration(t, raw)[0]); err != nil {
			t.Fatal(err)
		}
	}
	legacy := `INSERT INTO release_journal
 (operation_id,country,rel,mode,database_name,manifest_hash,dump_sha256,expected_active_db,manifest,step,state,message)
 VALUES (repeat('a',32),'vn','20990101','new','corpus_vn_20990101',repeat('b',64),repeat('c',64),'corpus_vn_20990102','{}','activated','complete','corpus activated');
 INSERT INTO retired_release(country,db_name,retired_at,retain_until,dropped_at,dump_sha256)
 VALUES('vn','corpus_vn_20990102',now(),now()+interval '7 days',NULL,repeat('c',64))
 ON CONFLICT(country,db_name) DO UPDATE SET retired_at=EXCLUDED.retired_at,
 retain_until=EXCLUDED.retain_until,dropped_at=NULL,dump_sha256=EXCLUDED.dump_sha256;`
	if _, err = conn.Exec(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, `INSERT INTO retired_release(country,db_name,retired_at,retain_until)
 VALUES('vn','invented_legacy_database',now(),now());
 INSERT INTO release_journal(country,rel,step,state,message,database_name)
 VALUES('vn','20990101','legacy','complete','invented','invented_legacy_database');
 INSERT INTO release_journal(country,rel,step,state,message,database_name,operation_id,mode)
 VALUES('vn','20990101_123','pruned','complete','invented','corpus_vn_20990101_123',repeat('b',32),'prune');`); err != nil {
		t.Fatal(err)
	}
	raw, err := migrations.FS.ReadFile("00012_frameworks_release_activation.sql")
	if err != nil {
		t.Fatal(err)
	}
	parts := splitMigration(t, raw)
	if _, err = conn.Exec(ctx, parts[0]); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	var law bool
	if err = conn.QueryRow(ctx, `SELECT bool_and(corpus_kind='law') FROM release_journal`).Scan(&law); err != nil || !law {
		t.Fatal("legacy journal changed kind")
	}
	if err = conn.QueryRow(ctx, `SELECT bool_and(corpus_kind='law') FROM retired_release`).Scan(&law); err != nil || !law {
		t.Fatal("legacy retired row changed kind")
	}
	if err = conn.QueryRow(ctx, `SELECT
 EXISTS(SELECT FROM retired_release WHERE db_name='invented_legacy_database' AND corpus_kind='law')
 AND EXISTS(SELECT FROM release_journal WHERE database_name='invented_legacy_database' AND corpus_kind='law')`).Scan(&law); err != nil || !law {
		t.Fatal("migration removed legacy rows")
	}
	for _, sql := range []string{
		`INSERT INTO retired_release(country,db_name,retired_at,retain_until) VALUES('vn','invented_new_database',now(),now())`,
		`UPDATE retired_release SET retired_at=now() WHERE db_name='invented_legacy_database'`,
		`INSERT INTO release_journal(country,rel,step,state,message,database_name) VALUES('vn','20990101','legacy','complete','invented','invented_new_database')`,
		`UPDATE release_journal SET message='invented update' WHERE database_name='invented_legacy_database'`,
	} {
		tx, beginErr := conn.Begin(ctx)
		if beginErr != nil {
			t.Fatal(beginErr)
		}
		_, writeErr := tx.Exec(ctx, sql)
		_ = tx.Rollback(ctx)
		var violation *pgconn.PgError
		if !errors.As(writeErr, &violation) || violation.Code != "23514" || (violation.ConstraintName != "retired_release_kind" && violation.ConstraintName != "release_journal_kind") {
			t.Fatal("kind constraint did not refuse a new or updated legacy name")
		}
	}
	for _, kind := range []string{"frameworks", "law", "other"} {
		for _, name := range []string{"frameworks_20990101", "frameworks_20990101_1", "frameworks_20990101_99", "frameworks_20990101_123", "corpus_vn_20990101", "corpus_vn_20990101_123", "auth"} {
			valid := kind == "frameworks" && (name == "frameworks_20990101" || name == "frameworks_20990101_1" || name == "frameworks_20990101_99") || kind == "law" && name == "corpus_vn_20990101"
			for _, sql := range []string{
				`INSERT INTO retired_release(country,db_name,retired_at,retain_until,corpus_kind) VALUES('vn',$1,now(),now(),$2) ON CONFLICT(country,db_name) DO UPDATE SET corpus_kind=EXCLUDED.corpus_kind`,
				`INSERT INTO release_journal(country,rel,step,state,message,operation_id,mode,database_name,corpus_kind) VALUES('vn','20990101','pruning','open','invented',repeat('d',32),'prune',$1,$2)`,
			} {
				tx, beginErr := conn.Begin(ctx)
				if beginErr != nil {
					t.Fatal(beginErr)
				}
				_, insertErr := tx.Exec(ctx, sql, name, kind)
				_ = tx.Rollback(ctx)
				if (insertErr == nil) != valid {
					t.Fatalf("kind=%s name=%s accepted=%t", kind, name, insertErr == nil)
				}
			}
		}
	}
	for _, country := range []string{"vn", "us"} {
		for _, name := range []string{"frameworks_20990101", "frameworks_20990101_99", "frameworks_20990101_123", "corpus_vn_20990101", "frameworks_20990101_bad"} {
			tx, beginErr := conn.Begin(ctx)
			if beginErr != nil {
				t.Fatal(beginErr)
			}
			_, insertErr := tx.Exec(ctx, `INSERT INTO frameworks_release VALUES($1,$2,now(),'invented')`, country, name)
			_ = tx.Rollback(ctx)
			valid := country == "vn" && (name == "frameworks_20990101" || name == "frameworks_20990101_99")
			if (insertErr == nil) != valid {
				t.Fatalf("pointer country=%s name=%s accepted=%t", country, name, insertErr == nil)
			}
		}
	}
	for _, change := range []string{
		`operation_id='invalid'`, `mode=NULL`, `mode='invalid'`, `database_name=NULL`,
		`database_name='auth'`, `expected_active_db='corpus_vn_20990101'`,
		`expected_active_db='frameworks_20990101_123'`,
		`manifest_hash=NULL`, `manifest_hash='invalid'`, `dump_sha256=NULL`,
		`dump_sha256='invalid'`, `manifest=NULL`, `manifest='[]'`,
	} {
		tx, beginErr := conn.Begin(ctx)
		if beginErr != nil {
			t.Fatal(beginErr)
		}
		_, insertErr := tx.Exec(ctx, `INSERT INTO release_journal(country,rel,step,state,message,operation_id,mode,database_name,corpus_kind,manifest_hash,dump_sha256,manifest)
 VALUES('vn','20990101','started','open','invented',repeat('e',32),'new','frameworks_20990101','frameworks',repeat('f',64),repeat('a',64),'{}')`)
		if insertErr != nil {
			_ = tx.Rollback(ctx)
			t.Fatal(insertErr)
		}
		_, updateErr := tx.Exec(ctx, `UPDATE release_journal SET `+change+` WHERE operation_id=repeat('e',32)`)
		_ = tx.Rollback(ctx)
		if updateErr == nil {
			t.Fatalf("operation accepted %s", change)
		}
	}
	if _, err = conn.Exec(ctx, parts[1]); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, legacy); err != nil {
		t.Fatal(err)
	}
}

func TestFrameworksActivationMigration_ActivatorGrant(t *testing.T) {
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
	if _, err = tx.Exec(ctx, `DO $$ BEGIN IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='activator') THEN CREATE ROLE activator NOLOGIN; END IF; END $$`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"00001_auth.sql", "00008_release_activation.sql", "00011_frameworks_release.sql", "00012_frameworks_release_activation.sql"} {
		raw, readErr := migrations.FS.ReadFile(name)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err = tx.Exec(ctx, splitMigration(t, raw)[0]); err != nil {
			t.Fatal(err)
		}
	}
	for _, privilege := range []string{"SELECT", "INSERT", "UPDATE", "DELETE", "TRUNCATE"} {
		var allowed bool
		if err = tx.QueryRow(ctx, `SELECT has_table_privilege('activator','frameworks_release',$1)`, privilege).Scan(&allowed); err != nil {
			t.Fatal(err)
		}
		if allowed != (privilege == "SELECT" || privilege == "INSERT" || privilege == "UPDATE") {
			t.Fatalf("unexpected %s grant", privilege)
		}
	}
	raw, err := migrations.FS.ReadFile("00012_frameworks_release_activation.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, splitMigration(t, raw)[1]); err != nil {
		t.Fatal(err)
	}
	var allowed bool
	if err = tx.QueryRow(ctx, `SELECT has_table_privilege('activator','frameworks_release','INSERT')`).Scan(&allowed); err != nil || allowed {
		t.Fatal("down retained activator grant")
	}
}
