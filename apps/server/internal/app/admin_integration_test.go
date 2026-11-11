//go:build integration

package app

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/internal/testpg"
)

func TestAdmin_PrintsTokenOnce(t *testing.T) {
	ctx := context.Background()
	url := testpg.NewDatabase(t)
	if err := migrate.Up(ctx, url); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SERVER_AUTH_DATABASE_URL", url)
	t.Setenv("SERVER_CORPUS_DATABASE_URL", "postgres://localhost/{database}")
	t.Setenv("SERVER_PUBLIC_URL", "http://localhost:8080")
	var out, stderr bytes.Buffer
	if err := run(ctx, []string{"admin", "account", "create", "--google-sub", "test-subject", "--email", "test@example.invalid"}, &out, &stderr); err != nil {
		t.Fatal(err)
	}
	id := strings.TrimSpace(out.String())
	out.Reset()
	if err := run(ctx, []string{"admin", "token", "create", "--account", id, "--name", "test"}, &out, &stderr); err != nil {
		t.Fatal(err)
	}
	token := strings.TrimSpace(out.String())
	if !strings.HasPrefix(token, "trc_") || strings.Count(out.String(), token) != 1 || stderr.Len() != 0 {
		t.Fatal("token output was not isolated")
	}
}
