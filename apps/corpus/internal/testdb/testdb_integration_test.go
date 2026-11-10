//go:build integration

package testdb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"testing"

	"danny.vn/trachuan/apps/corpus/migrations"
	"github.com/jackc/pgx/v5"
)

func TestNew_ConcurrentClonesShareOneReadyTemplate(t *testing.T) {
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, NewEmpty(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(ctx) })
	name := migrationTemplateName(t)
	t.Run("clones", func(t *testing.T) {
		for i := range 8 {
			t.Run(fmt.Sprintf("clone_%d", i), func(t *testing.T) {
				t.Parallel()
				New(t)
			})
		}
	})
	var count int
	var ready bool
	if err := admin.QueryRow(ctx, `SELECT count(*), COALESCE(bool_and(datistemplate), false) FROM pg_database WHERE datname = $1`, name).Scan(&count, &ready); err != nil {
		t.Fatal(err)
	}
	if count != 1 || !ready {
		t.Fatalf("template count=%d ready=%v", count, ready)
	}
}

func TestNew_KeepsOtherHashTemplates(t *testing.T) {
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, NewEmpty(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(ctx) })
	const other = "corpus_tpl_000000000000"
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{other}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{other}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Errorf("drop other template: %v", err)
		}
	})
	New(t)
	var exists bool
	if err := admin.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, other).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("other template was dropped")
	}
}

func migrationTemplateName(t *testing.T) string {
	t.Helper()
	files, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.New()
	for _, name := range files {
		data, err := migrations.FS.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = hash.Write(data)
	}
	return "corpus_tpl_" + hex.EncodeToString(hash.Sum(nil))[:12]
}

func TestNew_MigratedAndIsolated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	first, err := pgx.Connect(ctx, New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close(ctx) }()
	if _, execErr := first.Exec(ctx, `CREATE TABLE test_isolation (id int)`); execErr != nil {
		t.Fatal(execErr)
	}
	second, err := pgx.Connect(ctx, New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close(ctx) }()
	var migrated, isolated bool
	if err := second.QueryRow(ctx, `SELECT to_regclass('run') IS NOT NULL, to_regclass('test_isolation') IS NULL`).Scan(&migrated, &isolated); err != nil {
		t.Fatal(err)
	}
	if !migrated || !isolated {
		t.Fatalf("migrated=%v isolated=%v", migrated, isolated)
	}
}

func TestNewEmpty_HasNoMigrations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := pgx.Connect(ctx, NewEmpty(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close(ctx) }()
	var empty bool
	if err := db.QueryRow(ctx, `SELECT to_regclass('run') IS NULL`).Scan(&empty); err != nil {
		t.Fatal(err)
	}
	if !empty {
		t.Fatal("empty database contains run table")
	}
}
