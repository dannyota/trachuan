//go:build integration

// Package testdb creates isolated PostgreSQL databases for integration tests.
package testdb

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/migrate"
	"danny.vn/trachuan/apps/corpus/migrations"
	"github.com/jackc/pgx/v5"
)

// New creates an isolated database cloned from the migrated template.
func New(t *testing.T) string { return create(t, true) }

// NewEmpty creates an isolated database without migrations.
func NewEmpty(t *testing.T) string { return create(t, false) }

func create(t *testing.T, migrated bool) string {
	t.Helper()
	ctx := context.Background()
	adminURL := os.Getenv("CORPUS_TEST_ADMIN_URL")
	if adminURL == "" {
		adminURL = "postgres://trachuan@127.0.0.1:25433/postgres"
	}
	parsed, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal("invalid test database URL")
	}
	var random [12]byte
	if _, randErr := rand.Read(random[:]); randErr != nil {
		t.Fatalf("generate database name: %v", randErr)
	}
	name := "corpus_test_" + hex.EncodeToString(random[:])
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatal("connect to test database administrator")
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Errorf("drop test database: %v", err)
		}
		if err := admin.Close(ctx); err != nil {
			t.Errorf("close test database administrator: %v", err)
		}
	})
	query := "CREATE DATABASE " + pgx.Identifier{name}.Sanitize()
	if migrated {
		template, err := ensureTemplate(ctx, t, admin, parsed)
		if err != nil {
			t.Fatalf("prepare template: %v", err)
		}
		query += " TEMPLATE " + pgx.Identifier{template}.Sanitize()
	}
	if _, err := admin.Exec(ctx, query); err != nil {
		t.Fatalf("create test database: %v", err)
	}
	parsed.Path = "/" + name
	return parsed.String()
}

func ensureTemplate(ctx context.Context, t *testing.T, admin *pgx.Conn, adminURL *url.URL) (string, error) {
	files, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		return "", fmt.Errorf("list migrations: %w", err)
	}
	hash := sha256.New()
	for _, name := range files {
		data, readErr := migrations.FS.ReadFile(name)
		if readErr != nil {
			return "", fmt.Errorf("read migration: %w", readErr)
		}
		_, _ = hash.Write(data)
	}
	name := "corpus_tpl_" + hex.EncodeToString(hash.Sum(nil))[:12]
	ready, err := templateReady(ctx, admin, name)
	if err != nil {
		return "", err
	}
	if ready {
		return name, nil
	}
	if _, lockErr := admin.Exec(ctx, `SELECT pg_advisory_lock(723548171, hashtext($1))`, name); lockErr != nil {
		return "", fmt.Errorf("lock template: %w", lockErr)
	}
	defer func() {
		if _, unlockErr := admin.Exec(ctx, `SELECT pg_advisory_unlock(723548171, hashtext($1))`, name); unlockErr != nil {
			t.Errorf("unlock template: %v", unlockErr)
		}
	}()
	ready, err = templateReady(ctx, admin, name)
	if err != nil {
		return "", err
	}
	if ready {
		return name, nil
	}
	if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
		return "", fmt.Errorf("drop incomplete template: %w", err)
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		return "", fmt.Errorf("create template: %w", err)
	}
	templateURL := *adminURL
	templateURL.Path = "/" + name
	if err := migrate.Up(ctx, templateURL.String()); err != nil {
		if _, dropErr := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); dropErr != nil {
			return "", fmt.Errorf("migrate template: %w; drop template: %v", err, dropErr)
		}
		return "", fmt.Errorf("migrate template: %w", err)
	}
	if _, err := admin.Exec(ctx, "ALTER DATABASE "+pgx.Identifier{name}.Sanitize()+" IS_TEMPLATE true"); err != nil {
		return "", fmt.Errorf("mark template ready: %w", err)
	}
	return name, nil
}

func templateReady(ctx context.Context, admin *pgx.Conn, name string) (bool, error) {
	var ready bool
	if err := admin.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1 AND datistemplate)`, name).Scan(&ready); err != nil {
		return false, fmt.Errorf("find template: %w", err)
	}
	return ready, nil
}
