//go:build integration

// Package testpg creates isolated local PostgreSQL databases for integration tests.
package testpg

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"testing"

	"github.com/jackc/pgx/v5"
)

// Builder holds a test builder login and its extension template.
type Builder struct {
	URL      string
	Role     string
	Template string
}

func random(t *testing.T) string {
	t.Helper()
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("generate random identifier: %v", err)
	}
	return hex.EncodeToString(b[:])
}

func administrator(t *testing.T) (*pgx.Conn, *url.URL) {
	t.Helper()
	conn, parsed, err := OpenAdministrator(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(context.Background()); err != nil {
			t.Errorf("close test database administrator: %v", err)
		}
	})
	return conn, parsed
}

func databaseURL(base *url.URL, name string) string {
	copyURL := *base
	copyURL.Path = "/" + name
	return copyURL.String()
}

func install(t *testing.T, base *url.URL, name string, extensions []string) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, databaseURL(base, name))
	if err != nil {
		t.Fatal("connect to test database")
	}
	defer func() {
		if err := conn.Close(ctx); err != nil {
			t.Errorf("close test database: %v", err)
		}
	}()
	for _, extension := range extensions {
		if _, err := conn.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS "+pgx.Identifier{extension}.Sanitize()); err != nil {
			t.Fatalf("install test extension %q: %v", extension, err)
		}
	}
}

// NewDatabase creates an isolated database with the requested extensions.
func NewDatabase(t *testing.T, extensions ...string) string {
	t.Helper()
	ctx := context.Background()
	admin, base := administrator(t)
	name := "test_" + random(t)
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Errorf("drop test database: %v", err)
		}
	})
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create test database: %v", err)
	}
	install(t, base, name, extensions)
	return databaseURL(base, name)
}

// NewBuilder creates a non-superuser builder and an owned extension template.
func NewBuilder(t *testing.T) Builder {
	t.Helper()
	ctx := context.Background()
	admin, base := administrator(t)
	role := "test_builder_" + random(t)
	template := "test_template_" + random(t)
	password := random(t) + random(t)
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "ALTER DATABASE "+pgx.Identifier{template}.Sanitize()+" IS_TEMPLATE false"); err != nil {
			var exists bool
			if scanErr := admin.QueryRow(ctx, `SELECT EXISTS (SELECT FROM pg_database WHERE datname=$1)`, template).Scan(&exists); scanErr != nil || exists {
				t.Errorf("clear template flag: %v", err)
			}
		}
		if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{template}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Errorf("drop test template: %v", err)
		}
		if _, err := admin.Exec(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE usename=$1 AND pid <> pg_backend_pid()`, role); err != nil {
			t.Errorf("terminate builder connections: %v", err)
		}
		if _, err := admin.Exec(ctx, "DROP ROLE IF EXISTS "+pgx.Identifier{role}.Sanitize()); err != nil {
			t.Errorf("drop test builder: %v", err)
		}
	})
	if _, err := admin.Exec(ctx, "CREATE ROLE "+pgx.Identifier{role}.Sanitize()+" LOGIN CREATEDB NOSUPERUSER NOCREATEROLE PASSWORD '"+password+"'"); err != nil {
		t.Fatal("create test builder role")
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{template}.Sanitize()+" OWNER "+pgx.Identifier{role}.Sanitize()); err != nil {
		t.Fatalf("create test template: %v", err)
	}
	install(t, base, template, []string{"vector", "ltree"})
	if _, err := admin.Exec(ctx, "ALTER DATABASE "+pgx.Identifier{template}.Sanitize()+" IS_TEMPLATE true"); err != nil {
		t.Fatalf("mark test template: %v", err)
	}
	builderURL := *base
	builderURL.User = url.UserPassword(role, password)
	builderURL.Path = "/postgres"
	return Builder{URL: builderURL.String(), Role: role, Template: template}
}
