//go:build integration

package admin_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/admin"
	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
)

func TestAdmin_CredentialLifecycle(t *testing.T) {
	ctx := context.Background()
	url := testpg.NewDatabase(t)
	if err := migrate.Up(ctx, url); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal("connect to test database")
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })
	queries := store.New(conn)
	for _, name := range []string{"revoke", "block", "epoch", "expiry"} {
		t.Run(name, func(t *testing.T) {
			now := time.Now().UTC().Truncate(time.Second)
			a := &admin.Service{Store: queries, Clock: func() time.Time { return now }}
			account, err := a.CreateAccount(ctx, "test-subject-"+name, "test-"+name+"@example.test")
			if err != nil {
				t.Fatal(err)
			}
			lifetime := time.Hour
			if name == "expiry" {
				lifetime = 5 * time.Second
			}
			plain, err := a.CreateToken(ctx, account.ID, "cli", lifetime)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256([]byte(plain))
			hash := hex.EncodeToString(sum[:])
			var storedHash string
			var lastUsed *time.Time
			if err := conn.QueryRow(ctx, `SELECT token_hash, last_used_at FROM personal_token WHERE account_id = $1`, account.ID).Scan(&storedHash, &lastUsed); err != nil {
				t.Fatal("read stored token")
			}
			if storedHash != hash || lastUsed != nil {
				t.Fatal("unexpected stored token or usage timestamp")
			}
			v := &credential.Verifier{Store: queries, Clock: func() time.Time { return now }, Audience: "https://example.invalid/mcp"}
			p, err := v.Verify(ctx, plain)
			if err != nil || p.AccountID != account.ID || p.Scope != "mcp:read" || p.Audience != v.Audience {
				t.Fatal("issued credential did not authenticate")
			}
			if err := conn.QueryRow(ctx, `SELECT last_used_at FROM personal_token WHERE token_hash = $1`, hash).Scan(&lastUsed); err != nil || lastUsed == nil {
				t.Fatal("successful verification did not record usage")
			}
			window := 10 * time.Second
			switch name {
			case "revoke":
				if err := a.RevokeToken(ctx, hash); err != nil {
					t.Fatal(err)
				}
			case "block":
				if err := a.BlockAccount(ctx, account.ID); err != nil {
					t.Fatal(err)
				}
				row, err := queries.GetCredential(ctx, hash)
				if err != nil || row.BlockedAt == nil || row.RevokedAt == nil || row.AccountAuthEpoch != row.TokenAuthEpoch+1 {
					t.Fatal("block did not revoke and advance epoch")
				}
			case "epoch":
				if _, err := conn.Exec(ctx, `UPDATE account SET auth_epoch = auth_epoch + 1 WHERE id = $1`, account.ID); err != nil {
					t.Fatal("advance account epoch")
				}
			case "expiry":
				window = lifetime
			}
			now = now.Add(window - time.Nanosecond)
			if _, err := v.Verify(ctx, plain); err != nil {
				t.Fatal("cached token stopped working before its deadline")
			}
			now = now.Add(time.Nanosecond)
			if _, err := v.Verify(ctx, plain); !errors.Is(err, credential.ErrUnauthorized) {
				t.Fatal("credential survived revocation or expiry deadline")
			}
		})
	}

	t.Run("auth unavailable", func(t *testing.T) {
		a := &admin.Service{Store: queries}
		account, err := a.CreateAccount(ctx, "test-subject-unavailable", "test-unavailable@example.test")
		if err != nil {
			t.Fatal(err)
		}
		plain, err := a.CreateToken(ctx, account.ID, "cli", 0)
		if err != nil {
			t.Fatal(err)
		}
		if err := conn.Close(ctx); err != nil {
			t.Fatal("close test database connection")
		}
		v := &credential.Verifier{Store: queries, Audience: "https://example.invalid/mcp"}
		if _, err := v.Verify(ctx, plain); !errors.Is(err, credential.ErrUnavailable) {
			t.Fatal("unavailable store did not fail closed")
		}
	})
}
