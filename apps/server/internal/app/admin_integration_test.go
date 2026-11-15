//go:build integration

package app

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
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

func TestAdmin_UnblockRealStore(t *testing.T) {
	ctx := context.Background()
	url := testpg.NewDatabase(t)
	if err := migrate.Up(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := store.New(pool)
	account, err := q.CreateAccount(ctx, store.CreateAccountParams{GoogleSub: "unblock-subject", Email: "unblock@example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"account", "unblock", "--account", strconv.FormatInt(account.ID, 10)}
	if _, err := q.MarkAccountBlocked(ctx, account.ID); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runAdmin(ctx, args, q, &out); err != nil || out.Len() != 0 {
		t.Fatalf("unblock error=%v output=%q", err, out.String())
	}
	account, err = q.GetAccount(ctx, account.ID)
	if err != nil || account.BlockedAt != nil {
		t.Fatalf("account=%+v error=%v", account, err)
	}
	if _, err := q.MarkAccountDeleting(ctx, account.ID); err != nil {
		t.Fatal(err)
	}
	if err := runAdmin(ctx, args, q, &out); err == nil || !strings.Contains(err.Error(), "delet") {
		t.Fatalf("deleting error=%v", err)
	}
	account, err = q.GetAccount(ctx, account.ID)
	if err != nil || account.BlockedAt == nil {
		t.Fatalf("deleting account=%+v error=%v", account, err)
	}
	args[3] = "9223372036854775807"
	if err := runAdmin(ctx, args, q, &out); err == nil {
		t.Fatal("unknown account accepted")
	}
}
