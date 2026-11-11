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
