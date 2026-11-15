//go:build integration

package testdb

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestPruneOne_IsolatedTemplates(t *testing.T) {
	ctx := context.Background()
	adminURL := os.Getenv("CORPUS_TEST_ADMIN_URL")
	if adminURL == "" {
		adminURL = "postgres://trachuan@127.0.0.1:25433/postgres"
	}
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatal("connect to test database administrator")
	}
	t.Cleanup(func() { _ = admin.Close(ctx) })
	for _, tc := range []struct {
		name                       string
		marked, connected, current bool
		want                       string
		exists                     bool
	}{
		{"marked", true, false, false, "dropped", false},
		{"unmarked", false, false, false, "dropped", false},
		{"connected", false, true, false, "connected sessions", true},
		{"current", false, false, true, "current migration", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var random [6]byte
			if _, err := rand.Read(random[:]); err != nil {
				t.Fatal(err)
			}
			name := "corpus_tpl_" + hex.EncodeToString(random[:])
			identifier := pgx.Identifier{name}.Sanitize()
			current := ""
			if tc.current {
				current = name
			} else {
				if _, err := admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if _, err := admin.Exec(ctx, "ALTER DATABASE "+identifier+" IS_TEMPLATE false"); err != nil {
						var exists bool
						if scanErr := admin.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, name).Scan(&exists); scanErr != nil || exists {
							t.Errorf("unmark cleanup template: %v", err)
						}
					}
					if err := dropIfExists(ctx, admin, name); err != nil {
						t.Errorf("drop cleanup template: %v", err)
					}
				})
			}
			if tc.marked {
				if _, err := admin.Exec(ctx, "ALTER DATABASE "+identifier+" IS_TEMPLATE true"); err != nil {
					t.Fatal(err)
				}
			}
			if tc.connected {
				parsed, err := url.Parse(adminURL)
				if err != nil {
					t.Fatal("invalid test database URL")
				}
				parsed.Path = "/" + name
				conn, err := pgx.Connect(ctx, parsed.String())
				if err != nil {
					t.Fatal("connect to test template")
				}
				t.Cleanup(func() { _ = conn.Close(ctx) })
				if err := refuseActiveTests(ctx, admin); err == nil {
					t.Error("active test session was not refused")
				}
			}
			var out bytes.Buffer
			if err := pruneOne(ctx, admin, &out, name, current); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Errorf("output = %q, want %q", out.String(), tc.want)
			}
			var exists bool
			if err := admin.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, name).Scan(&exists); err != nil {
				t.Fatal(err)
			}
			if exists != tc.exists {
				t.Errorf("exists = %t, want %t", exists, tc.exists)
			}
		})
	}
}
