//go:build integration

// Package testdb creates isolated PostgreSQL databases for integration tests.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

// New creates a database and registers its removal with t.
func New(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	adminURL := os.Getenv("CORPUS_TEST_ADMIN_URL")
	if adminURL == "" {
		adminURL = "postgres://traquydinh@127.0.0.1:5432/postgres"
	}
	parsed, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal("invalid test database URL")
	}
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatalf("generate database name: %v", err)
	}
	name := "corpus_test_" + hex.EncodeToString(random[:])
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatal("connect to test database administrator")
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		_ = admin.Close(ctx)
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Errorf("drop test database: %v", err)
		}
		if err := admin.Close(ctx); err != nil {
			t.Errorf("close test database administrator: %v", err)
		}
	})
	parsed.Path = "/" + name
	return parsed.String()
}
