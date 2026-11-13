//go:build integration

package stage

import (
	"context"
	"database/sql"
	"slices"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"danny.vn/trachuan/apps/corpus/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestRunItemForeignKeys_HaveLeadingIndex(t *testing.T) {
	ctx := context.Background()
	db, provider := openMigrations(t)
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	fks := columnLists(t, db, `SELECT c.conrelid::regclass::text, c.conname, array_to_string(ARRAY(SELECT a.attname FROM unnest(c.conkey) WITH ORDINALITY AS k(n, o) JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = k.n ORDER BY k.o), ',') FROM pg_constraint c WHERE c.contype = 'f' AND c.confrelid = 'run_item'::regclass`)
	if len(fks) == 0 {
		t.Fatal("no foreign keys reference run_item")
	}
	indexes := columnLists(t, db, `SELECT i.indrelid::regclass::text, i.indexrelid::regclass::text, array_to_string(ARRAY(SELECT a.attname FROM unnest(i.indkey) WITH ORDINALITY AS k(n, o) JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = k.n ORDER BY k.o), ',') FROM pg_index i`)
	for _, fk := range fks {
		if !slices.ContainsFunc(indexes, func(ix columnList) bool { return ix.table == fk.table && hasPrefix(ix.columns, fk.columns) }) {
			t.Errorf("%s.%s (%v) has no index leading with its columns", fk.table, fk.name, fk.columns)
		}
	}
}

func TestRunItemForeignKeyIndexMigration_UpAndDown(t *testing.T) {
	ctx := context.Background()
	db, provider := openMigrations(t)
	if _, err := provider.UpTo(ctx, 11); err != nil {
		t.Fatal(err)
	}
	names := []string{"run_membership_evidence_item_idx", "run_seed_observation_item_idx"}
	for _, name := range names {
		var exists bool
		if err := db.QueryRowContext(ctx, `SELECT to_regclass($1) IS NOT NULL`, name).Scan(&exists); err != nil || !exists {
			t.Fatalf("up: %s exists=%v err=%v", name, exists, err)
		}
	}
	if _, err := provider.Down(ctx); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		var exists bool
		if err := db.QueryRowContext(ctx, `SELECT to_regclass($1) IS NOT NULL`, name).Scan(&exists); err != nil || exists {
			t.Fatalf("down: %s exists=%v err=%v", name, exists, err)
		}
	}
}

type columnList struct {
	table, name string
	columns     []string
}

func openMigrations(t *testing.T) (*sql.DB, *goose.Provider) {
	t.Helper()
	cfg, err := pgx.ParseConfig(testdb.NewEmpty(t))
	if err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDB(*cfg)
	t.Cleanup(func() { _ = db.Close() })
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	return db, provider
}

func columnLists(t *testing.T, db *sql.DB, query string) []columnList {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []columnList
	for rows.Next() {
		var c columnList
		var joined string
		if err := rows.Scan(&c.table, &c.name, &joined); err != nil {
			t.Fatal(err)
		}
		c.columns = strings.Split(joined, ",")
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func hasPrefix(columns, prefix []string) bool {
	return len(columns) >= len(prefix) && slices.Equal(columns[:len(prefix)], prefix)
}
