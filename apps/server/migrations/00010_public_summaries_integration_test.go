//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"danny.vn/trachuan/apps/server/internal/publicsummary"
	"danny.vn/trachuan/apps/server/migrations"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
)

func TestSummaryMigration_DefaultAndApprovalConstraint(t *testing.T) {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testpg.NewDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	base, err := migrations.FS.ReadFile("00001_auth.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, splitMigration(t, base)[0]); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, `INSERT INTO corpus_release VALUES ('vn','corpus_vn_20990101',now(),'fixture')`); err != nil {
		t.Fatal(err)
	}
	raw, err := migrations.FS.ReadFile("00010_public_summaries.sql")
	if err != nil {
		t.Fatal(err)
	}
	parts := splitMigration(t, raw)
	if _, err = conn.Exec(ctx, parts[0]); err != nil {
		t.Fatal(err)
	}
	var projection int
	var approved bool
	if err = conn.QueryRow(ctx, `SELECT summary_projection,headings_approved FROM corpus_release`).Scan(&projection, &approved); err != nil || projection != 0 || approved {
		t.Fatalf("legacy default: %d %v %v", projection, approved, err)
	}
	if _, err = conn.Exec(ctx, `UPDATE corpus_release SET headings_approved=true`); err == nil {
		t.Fatal("approval without capability accepted")
	}
	if _, err = conn.Exec(ctx, `UPDATE corpus_release SET summary_database=active_db,summary_projection=1,headings_approved=true`); err != nil {
		t.Fatal(err)
	}
	metadata := publicsummary.Metadata(conn)
	active, err := metadata(ctx)
	if err != nil || active.Projection != 1 || !active.HeadingsApproved {
		t.Fatalf("approved: %+v %v", active, err)
	}
	if _, err = conn.Exec(ctx, `UPDATE corpus_release SET active_db='corpus_vn_20990102'`); err != nil {
		t.Fatal(err)
	}
	active, err = metadata(ctx)
	if err != nil || active.Projection != 0 || active.HeadingsApproved {
		t.Fatalf("manual pointer inherited approval: %+v %v", active, err)
	}
	if _, err = conn.Exec(ctx, parts[1]); err != nil {
		t.Fatal(err)
	}
}
