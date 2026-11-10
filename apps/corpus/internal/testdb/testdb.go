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
	"strings"
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
		if _, err := admin.Exec(ctx, `SELECT pg_advisory_lock(723548171)`); err != nil {
			t.Fatalf("lock template: %v", err)
		}
		defer func() {
			if _, err := admin.Exec(ctx, `SELECT pg_advisory_unlock(723548171)`); err != nil {
				t.Errorf("unlock template: %v", err)
			}
		}()
		template, err := ensureTemplate(ctx, admin, parsed)
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

func ensureTemplate(ctx context.Context, admin *pgx.Conn, adminURL *url.URL) (string, error) {
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
	var exists bool
	if queryErr := admin.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, name).Scan(&exists); queryErr != nil {
		return "", fmt.Errorf("find template: %w", queryErr)
	}
	if exists {
		return name, nil
	}
	rows, err := admin.Query(ctx, `SELECT datname FROM pg_database WHERE datname LIKE 'corpus\_tpl\_%' ESCAPE '\'`)
	if err != nil {
		return "", fmt.Errorf("list templates: %w", err)
	}
	var old []string
	for rows.Next() {
		var value string
		if scanErr := rows.Scan(&value); scanErr != nil {
			rows.Close()
			return "", fmt.Errorf("read template name: %w", scanErr)
		}
		if strings.HasPrefix(value, "corpus_tpl_") {
			old = append(old, value)
		}
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		rows.Close()
		return "", fmt.Errorf("read templates: %w", rowsErr)
	}
	rows.Close()
	for _, value := range old {
		if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{value}.Sanitize()+" WITH (FORCE)"); err != nil {
			return "", fmt.Errorf("drop old template: %w", err)
		}
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
	return name, nil
}
