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

func TestFrameworksActivationMigration_DownRefusesCommittedFrameworksState(t *testing.T) {
	for _, tc := range []struct{ name, seed string }{
		{"journal", `INSERT INTO release_journal(country,rel,step,state,message,operation_id,mode,database_name,corpus_kind)
 VALUES('vn','20990101','pruning','open','invented',repeat('a',32),'prune','frameworks_20990101','frameworks')`},
		{"retirement", `INSERT INTO retired_release(country,db_name,retired_at,retain_until,corpus_kind)
 VALUES('vn','frameworks_20990101',now(),now(),'frameworks')`},
		{"pointer", `INSERT INTO frameworks_release VALUES('vn','frameworks_20990101',now(),'invented')`},
		{"none", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			conn, err := pgx.Connect(ctx, testpg.NewDatabase(t))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = conn.Close(context.Background()) }()
			var down string
			for _, name := range []string{"00001_auth.sql", "00008_release_activation.sql", "00011_frameworks_release.sql", "00012_frameworks_release_activation.sql"} {
				raw, readErr := migrations.FS.ReadFile(name)
				if readErr != nil {
					t.Fatal(readErr)
				}
				parts := splitMigration(t, raw)
				if _, err = conn.Exec(ctx, parts[0]); err != nil {
					t.Fatal(err)
				}
				down = parts[1]
			}
			if tc.seed != "" {
				if _, err = conn.Exec(ctx, tc.seed); err != nil {
					t.Fatal(err)
				}
			}
			var grantedBefore bool
			grantSQL := `SELECT coalesce((SELECT has_table_privilege(oid,'frameworks_release','INSERT') FROM pg_roles WHERE rolname='activator'),false)`
			if err = conn.QueryRow(ctx, grantSQL).Scan(&grantedBefore); err != nil {
				t.Fatal(err)
			}
			_, downErr := conn.Exec(ctx, down)
			if tc.seed == "" {
				if downErr != nil {
					t.Fatal(downErr)
				}
				var columns int
				if err = conn.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name IN ('release_journal','retired_release') AND column_name='corpus_kind'`).Scan(&columns); err != nil || columns != 0 {
					t.Fatal("down retained kind columns without frameworks state")
				}
				return
			}
			var refused *pgconn.PgError
			if !errors.As(downErr, &refused) || refused.Code != "55000" {
				t.Fatal("down did not refuse committed frameworks state before changing schema")
			}
			var states int
			if err = conn.QueryRow(ctx, `SELECT (SELECT count(*) FROM release_journal WHERE corpus_kind='frameworks')
 +(SELECT count(*) FROM retired_release WHERE corpus_kind='frameworks')+(SELECT count(*) FROM frameworks_release)`).Scan(&states); err != nil || states != 1 {
				t.Fatal("refused down changed frameworks state")
			}
			var grantedAfter bool
			if err = conn.QueryRow(ctx, grantSQL).Scan(&grantedAfter); err != nil || grantedAfter != grantedBefore {
				t.Fatal("refused down changed activator grant")
			}
			var checks int
			if err = conn.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE conname IN ('release_journal_kind','retired_release_kind','frameworks_release_country','frameworks_release_active_db')`).Scan(&checks); err != nil || checks != 4 {
				t.Fatal("refused down changed constraints")
			}
		})
	}
}
