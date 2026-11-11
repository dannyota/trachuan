//go:build integration

package store_test

import (
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestStore_OAuthRetentionBoundariesAndBatch(t *testing.T) {
	ctx, conn, q, _, grant := oauthFixture(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	hashes := []string{strings.Repeat("a", 64), strings.Repeat("b", 64)}
	for _, hash := range hashes {
		oauthToken(t, ctx, q, grant, hash, now.Add(24*time.Hour))
	}
	if _, err := conn.Exec(ctx, `UPDATE oauth_grant SET revoked_at = $2 WHERE id = $1`, grant, now.Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	period := pgtype.Interval{Microseconds: int64(24 * time.Hour / time.Microsecond), Valid: true}
	args := store.DeleteExpiredOAuthTokensParams{AsOf: now.Add(-time.Microsecond), Retention: period, BatchLimit: 1}
	if n, err := q.DeleteExpiredOAuthTokens(ctx, args); err != nil || n != 0 {
		t.Fatalf("before boundary: deleted=%d err=%v", n, err)
	}
	args.AsOf = now
	for range 2 {
		if n, err := q.DeleteExpiredOAuthTokens(ctx, args); err != nil || n != 1 {
			t.Fatalf("at boundary: deleted=%d err=%v", n, err)
		}
	}
	if n, err := q.DeleteExpiredOAuthTokens(ctx, args); err != nil || n != 0 {
		t.Fatalf("after batches: deleted=%d err=%v", n, err)
	}
}

func TestStore_OAuthRetentionDeduplicatesTokenBatch(t *testing.T) {
	ctx, conn, q, _, grant := oauthFixture(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	other := webAccount(t, ctx, conn, "retention-other")
	otherGrant, err := q.UpsertGrant(ctx, store.UpsertGrantParams{AccountID: other, ClientID: "client", Scope: "mcp:read", Resource: "https://example.invalid/mcp"})
	if err != nil {
		t.Fatal(err)
	}
	for _, char := range []string{"a", "b"} {
		oauthToken(t, ctx, q, grant, strings.Repeat(char, 64), now.Add(-72*time.Hour))
	}
	for _, char := range []string{"c", "d", "e", "f"} {
		oauthToken(t, ctx, q, otherGrant.ID, strings.Repeat(char, 64), now.Add(24*time.Hour))
	}
	if _, err := conn.Exec(ctx, `UPDATE oauth_grant SET revoked_at = CASE WHEN id = $1 THEN $3 ELSE $4 END WHERE id IN ($1, $2)`, grant, otherGrant.ID, now.Add(-72*time.Hour), now.Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	args := store.DeleteExpiredOAuthTokensParams{AsOf: now, Retention: pgtype.Interval{Microseconds: int64(24 * time.Hour / time.Microsecond), Valid: true}, BatchLimit: 4}
	if n, err := q.DeleteExpiredOAuthTokens(ctx, args); err != nil || n != 4 {
		t.Fatalf("first batch deleted=%d, want 4: %v", n, err)
	}
	if n, err := q.DeleteExpiredOAuthTokens(ctx, args); err != nil || n != 2 {
		t.Fatalf("second batch deleted=%d, want 2: %v", n, err)
	}
}
