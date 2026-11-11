//go:build integration

package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

func webAccount(t *testing.T, ctx context.Context, conn *pgx.Conn, sub string) int64 {
	t.Helper()
	account, err := store.New(conn).CreateAccount(ctx, store.CreateAccountParams{GoogleSub: sub, Email: "test@example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	return account.ID
}

func webSession(t *testing.T, ctx context.Context, conn *pgx.Conn, account int64, hash string) int64 {
	t.Helper()
	session, err := store.New(conn).CreateSession(ctx, store.CreateSessionParams{
		TokenHash: hash, AccountID: account, CsrfSecret: "secret", ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	return session.ID
}

func TestStore_ClaimOnce(t *testing.T) {
	ctx, conn, q := setup(t)
	hash := strings.Repeat("a", 64)
	_, err := q.CreateLoginTransaction(ctx, store.CreateLoginTransactionParams{
		HandleHash: hash, State: "state", Nonce: "nonce", PkceVerifier: "verifier",
		NextPath: "/", RedirectUri: "https://example.invalid", ExpiresAt: time.Now().Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	other, err := pgx.ConnectConfig(ctx, conn.Config().Copy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = other.Close(ctx) })
	results := make(chan error, 2)
	start := make(chan struct{})
	for _, queries := range []*store.Queries{q, store.New(other)} {
		go func(queries *store.Queries) {
			<-start
			_, err := queries.ClaimLoginTransaction(ctx, hash)
			results <- err
		}(queries)
	}
	close(start)
	wins, misses := 0, 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			wins++
		case errors.Is(err, pgx.ErrNoRows):
			misses++
		default:
			t.Fatal(err)
		}
	}
	if wins != 1 || misses != 1 {
		t.Fatalf("wins=%d misses=%d", wins, misses)
	}
}

func TestStore_SessionJoin(t *testing.T) {
	ctx, conn, q := setup(t)
	id := webAccount(t, ctx, conn, "one")
	hash := strings.Repeat("b", 64)
	webSession(t, ctx, conn, id, hash)
	if _, err := conn.Exec(ctx, `UPDATE account SET blocked_at = now(), delete_requested_at = now(), auth_epoch = 2 WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	row, err := q.GetSession(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	if row.AccountID != id || row.BlockedAt == nil || row.DeleteRequestedAt == nil || row.AccountAuthEpoch != 2 || row.SessionAuthEpoch != 0 {
		t.Fatalf("unexpected session: %+v", row)
	}
}

func TestStore_TouchAtMostHourly(t *testing.T) {
	ctx, conn, q := setup(t)
	id := webAccount(t, ctx, conn, "one")
	hash := strings.Repeat("c", 64)
	webSession(t, ctx, conn, id, hash)
	if count, err := q.TouchSession(ctx, hash); err != nil || count != 0 {
		t.Fatalf("fresh touch: count=%d err=%v", count, err)
	}
	if _, err := conn.Exec(ctx, `UPDATE web_session SET created_at = now() - interval '89 days', last_seen_at = now() - interval '2 hours' WHERE token_hash = $1`, hash); err != nil {
		t.Fatal(err)
	}
	if count, err := q.TouchSession(ctx, hash); err != nil || count != 1 {
		t.Fatalf("old touch: count=%d err=%v", count, err)
	}
	var capped bool
	if err := conn.QueryRow(ctx, `SELECT expires_at = created_at + interval '90 days' FROM web_session WHERE token_hash = $1`, hash).Scan(&capped); err != nil || !capped {
		t.Fatalf("expiry cap=%v err=%v", capped, err)
	}
	if count, err := q.TouchSession(ctx, hash); err != nil || count != 0 {
		t.Fatalf("repeat touch: count=%d err=%v", count, err)
	}
}

func TestStore_TouchInactiveSession(t *testing.T) {
	for _, tc := range []struct {
		name   string
		update string
	}{
		{"expired", `expires_at = now() - interval '1 day'`},
		{"revoked", `revoked_at = now()`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, conn, q := setup(t)
			id := webAccount(t, ctx, conn, "one")
			hash := strings.Repeat("c", 64)
			webSession(t, ctx, conn, id, hash)
			if _, err := conn.Exec(ctx, `UPDATE web_session SET last_seen_at = now() - interval '2 hours', `+tc.update+` WHERE token_hash = $1`, hash); err != nil {
				t.Fatal(err)
			}
			before, err := q.GetSession(ctx, hash)
			if err != nil {
				t.Fatal(err)
			}
			count, err := q.TouchSession(ctx, hash)
			if err != nil || count != 0 {
				t.Fatalf("touch: count=%d err=%v", count, err)
			}
			after, err := q.GetSession(ctx, hash)
			if err != nil {
				t.Fatal(err)
			}
			if !after.ExpiresAt.Equal(before.ExpiresAt) || !after.LastSeenAt.Equal(before.LastSeenAt) {
				t.Fatalf("touch changed expiry or last seen: before=%+v after=%+v", before, after)
			}
		})
	}
}

func TestStore_RequestDeletionRevokesAll(t *testing.T) {
	ctx, conn, q := setup(t)
	id := webAccount(t, ctx, conn, "one")
	webSession(t, ctx, conn, id, strings.Repeat("d", 64))
	webSession(t, ctx, conn, id, strings.Repeat("f", 64))
	other := webAccount(t, ctx, conn, "other")
	webSession(t, ctx, conn, other, strings.Repeat("a", 64))
	for _, hash := range []string{"e", "b"} {
		if _, err := q.CreatePersonalToken(ctx, store.CreatePersonalTokenParams{TokenHash: strings.Repeat(hash, 64), AccountID: id, Name: "test", ExpiresAt: time.Now().Add(24 * time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := q.RequestDeletion(ctx, id); err != nil {
		t.Fatal(err)
	}
	var ok bool
	if err := conn.QueryRow(ctx, `SELECT blocked_at IS NOT NULL AND delete_requested_at IS NOT NULL AND auth_epoch = 1 AND (SELECT bool_and(revoked_at IS NOT NULL) FROM web_session WHERE account_id = $1) AND (SELECT bool_and(revoked_at IS NOT NULL) FROM personal_token WHERE account_id = $1) FROM account WHERE id = $1`, id).Scan(&ok); err != nil || !ok {
		t.Fatalf("deletion state=%v err=%v", ok, err)
	}
	if session, err := q.GetSession(ctx, strings.Repeat("a", 64)); err != nil || session.RevokedAt != nil {
		t.Fatalf("other session revoked=%v err=%v", session.RevokedAt, err)
	}
}

func TestStore_PurgeAfterSevenDays(t *testing.T) {
	ctx, conn, q := setup(t)
	old := webAccount(t, ctx, conn, "old")
	fresh := webAccount(t, ctx, conn, "fresh")
	webSession(t, ctx, conn, old, strings.Repeat("f", 64))
	if _, err := conn.Exec(ctx, `UPDATE account SET delete_requested_at = now() - CASE WHEN id = $1 THEN interval '8 days' ELSE interval '6 days' END WHERE id IN ($1, $2)`, old, fresh); err != nil {
		t.Fatal(err)
	}
	if count, err := q.PurgeDeletedAccounts(ctx, time.Now()); err != nil || count != 1 {
		t.Fatalf("purged=%d err=%v", count, err)
	}
	if _, err := q.GetAccount(ctx, old); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("old account error=%v", err)
	}
	if _, err := q.GetAccount(ctx, fresh); err != nil {
		t.Fatal(err)
	}
}

func TestStore_OwnRowsOnly(t *testing.T) {
	ctx, conn, q := setup(t)
	owner := webAccount(t, ctx, conn, "owner")
	other := webAccount(t, ctx, conn, "other")
	session := webSession(t, ctx, conn, other, strings.Repeat("a", 64))
	token, err := q.CreatePersonalToken(ctx, store.CreatePersonalTokenParams{TokenHash: strings.Repeat("b", 64), AccountID: other, Name: "test", ExpiresAt: time.Now().Add(24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if token.ID == 0 {
		t.Fatal("token id is zero")
	}
	if count, err := q.RevokeOwnSession(ctx, store.RevokeOwnSessionParams{AccountID: owner, ID: session}); err != nil || count != 0 {
		t.Fatalf("revoke session: count=%d err=%v", count, err)
	}
	if digest, err := q.RevokeOwnToken(ctx, store.RevokeOwnTokenParams{AccountID: owner, ID: token.ID}); !errors.Is(err, pgx.ErrNoRows) || digest != "" {
		t.Fatalf("revoke token: hash returned=%t err=%v", digest != "", err)
	}
	if sessions, err := q.ListSessions(ctx, owner); err != nil || len(sessions) != 0 {
		t.Fatalf("sessions=%v err=%v", sessions, err)
	}
	if tokens, err := q.ListTokens(ctx, owner); err != nil || len(tokens) != 0 {
		t.Fatalf("tokens=%v err=%v", tokens, err)
	}
	if count, err := q.RevokeOwnSession(ctx, store.RevokeOwnSessionParams{AccountID: other, ID: session}); err != nil || count != 1 {
		t.Fatalf("revoke own session: count=%d err=%v", count, err)
	}
	if digest, err := q.RevokeOwnToken(ctx, store.RevokeOwnTokenParams{AccountID: other, ID: token.ID}); err != nil || digest != token.TokenHash {
		t.Fatalf("revoke own token: hash matches=%t err=%v", digest == token.TokenHash, err)
	}
}
