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
	"github.com/jackc/pgx/v5/pgtype"
)

func oauthFixture(t *testing.T) (context.Context, *pgx.Conn, *store.Queries, int64, int64) {
	t.Helper()
	ctx, conn, q := setup(t)
	account := webAccount(t, ctx, conn, "oauth-owner")
	_, err := q.CreateClient(ctx, store.CreateClientParams{ClientID: "client", Kind: "dcr", Name: "Client", RedirectUris: []string{"https://example.invalid/cb"}})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := q.UpsertGrant(ctx, store.UpsertGrantParams{AccountID: account, ClientID: "client", Scope: "mcp:read", Resource: "https://example.invalid/mcp"})
	if err != nil {
		t.Fatal(err)
	}
	return ctx, conn, q, account, grant.ID
}

func oauthCode(t *testing.T, ctx context.Context, q *store.Queries, grant int64, hash string, expiry time.Time) {
	t.Helper()
	_, err := q.CreateCode(ctx, store.CreateCodeParams{GrantID: grant, CodeHash: hash, RedirectUri: "https://example.invalid/cb", Resource: "https://example.invalid/mcp", CodeChallenge: "challenge", Scope: "mcp:read", ExpiresAt: expiry})
	if err != nil {
		t.Fatal(err)
	}
}

func oauthToken(t *testing.T, ctx context.Context, q *store.Queries, grant int64, hash string, expiry time.Time) {
	t.Helper()
	_, err := q.CreateTokenFamily(ctx, store.CreateTokenFamilyParams{GrantID: grant, TokenHash: hash, Kind: "refresh", FamilyID: hash, Resource: "https://example.invalid/mcp", Scope: "mcp:read", ExpiresAt: expiry, FamilyExpiresAt: expiry})
	if err != nil {
		t.Fatal(err)
	}
}

func TestStore_ClaimRequestOnce(t *testing.T) {
	ctx, _, q, account, _ := oauthFixture(t)
	handle, browser := strings.Repeat("a", 64), strings.Repeat("b", 64)
	_, err := q.CreateRequest(ctx, store.CreateRequestParams{HandleHash: handle, BrowserHash: browser, ClientID: "client", RedirectUri: "https://example.invalid/cb", State: "state", CodeChallenge: "challenge", Resource: "https://example.invalid/mcp", Scope: "mcp:read", ExpiresAt: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	args := store.ClaimRequestParams{HandleHash: handle, BrowserHash: strings.Repeat("c", 64), AccountID: &account}
	if _, err = q.ClaimRequest(ctx, args); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("wrong browser: %v", err)
	}
	args.BrowserHash = browser
	if _, err = q.ClaimRequest(ctx, args); err != nil {
		t.Fatal(err)
	}
	if _, err = q.ClaimRequest(ctx, args); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("second claim: %v", err)
	}
}

func TestStore_ConsumeCodeOnce(t *testing.T) {
	ctx, _, q, _, grant := oauthFixture(t)
	hash := strings.Repeat("a", 64)
	oauthCode(t, ctx, q, grant, hash, time.Now().Add(time.Minute))
	first, err := q.ConsumeCode(ctx, hash)
	if err != nil || first.Replay != false {
		t.Fatalf("first consume replay=%v err=%v", first.Replay, err)
	}
	second, err := q.ConsumeCode(ctx, hash)
	if err != nil || second.Replay != true {
		t.Fatalf("second consume replay=%v err=%v", second.Replay, err)
	}
}

func TestStore_RotateRefreshOnce(t *testing.T) {
	ctx, _, q, _, grant := oauthFixture(t)
	hash := strings.Repeat("a", 64)
	oauthToken(t, ctx, q, grant, hash, time.Now().Add(time.Hour))
	first, err := q.RotateRefresh(ctx, hash)
	if err != nil || first.Reuse != false {
		t.Fatalf("first rotation reuse=%v err=%v", first.Reuse, err)
	}
	second, err := q.RotateRefresh(ctx, hash)
	if err != nil || second.Reuse != true {
		t.Fatalf("second rotation reuse=%v err=%v", second.Reuse, err)
	}
}

func TestStore_RevokeGrantRevokesAll(t *testing.T) {
	ctx, conn, q, _, grant := oauthFixture(t)
	oauthCode(t, ctx, q, grant, strings.Repeat("a", 64), time.Now().Add(time.Hour))
	oauthToken(t, ctx, q, grant, strings.Repeat("b", 64), time.Now().Add(time.Hour))
	if _, err := q.RevokeGrant(ctx, grant); err != nil {
		t.Fatal(err)
	}
	var revoked bool
	err := conn.QueryRow(ctx, `SELECT g.revoked_at IS NOT NULL AND c.revoked_at IS NOT NULL AND t.revoked_at IS NOT NULL FROM oauth_grant g JOIN oauth_code c ON c.grant_id = g.id JOIN oauth_token t ON t.grant_id = g.id WHERE g.id = $1`, grant).Scan(&revoked)
	if err != nil || !revoked {
		t.Fatalf("revoked=%v err=%v", revoked, err)
	}
}

func TestStore_OwnGrantsOnly(t *testing.T) {
	ctx, conn, q, owner, grant := oauthFixture(t)
	other := webAccount(t, ctx, conn, "oauth-other")
	if rows, err := q.ListGrants(ctx, other); err != nil || len(rows) != 0 {
		t.Fatalf("other grants=%v err=%v", rows, err)
	}
	if _, err := q.RevokeOwnGrant(ctx, store.RevokeOwnGrantParams{AccountID: other, ID: grant}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("other revoke: %v", err)
	}
	if rows, err := q.ListGrants(ctx, owner); err != nil || len(rows) != 1 {
		t.Fatalf("owner grants=%v err=%v", rows, err)
	}
	if _, err := q.RevokeOwnGrant(ctx, store.RevokeOwnGrantParams{AccountID: owner, ID: grant}); err != nil {
		t.Fatal(err)
	}
}

func TestStore_RetentionDeletes(t *testing.T) {
	ctx, conn, q, _, grant := oauthFixture(t)
	now := time.Now().UTC()
	oauthCode(t, ctx, q, grant, strings.Repeat("a", 64), now.Add(-48*time.Hour))
	oauthToken(t, ctx, q, grant, strings.Repeat("b", 64), now.Add(-48*time.Hour))
	_, err := q.CreateRequest(ctx, store.CreateRequestParams{HandleHash: strings.Repeat("c", 64), BrowserHash: strings.Repeat("d", 64), ClientID: "client", RedirectUri: "https://example.invalid/cb", State: "state", CodeChallenge: "challenge", Resource: "https://example.invalid/mcp", Scope: "mcp:read", ExpiresAt: now.Add(-time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	period := pgtype.Interval{Microseconds: int64(24 * time.Hour / time.Microsecond), Valid: true}
	if n, err := q.DeleteExpiredOAuthRequests(ctx, store.DeleteExpiredOAuthRequestsParams{AsOf: now, Retention: period, BatchLimit: 1}); err != nil || n != 1 {
		t.Fatalf("requests=%d err=%v", n, err)
	}
	if n, err := q.DeleteExpiredOAuthCodes(ctx, store.DeleteExpiredOAuthCodesParams{AsOf: now, Retention: period, BatchLimit: 1}); err != nil || n != 1 {
		t.Fatalf("codes=%d err=%v", n, err)
	}
	if n, err := q.DeleteExpiredOAuthTokens(ctx, store.DeleteExpiredOAuthTokensParams{AsOf: now, Retention: period, BatchLimit: 1}); err != nil || n != 1 {
		t.Fatalf("tokens=%d err=%v", n, err)
	}
	if n, err := q.DeleteRetiredOAuthGrants(ctx, store.DeleteRetiredOAuthGrantsParams{AsOf: now, Retention: period, BatchLimit: 1}); err != nil || n != 0 {
		t.Fatalf("fresh grant=%d err=%v", n, err)
	}
	if _, err := conn.Exec(ctx, `UPDATE oauth_grant SET retirement_anchor = $2 WHERE id = $1`, grant, now.Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if n, err := q.DeleteRetiredOAuthGrants(ctx, store.DeleteRetiredOAuthGrantsParams{AsOf: now, Retention: period, BatchLimit: 1}); err != nil || n != 1 {
		t.Fatalf("old grant=%d err=%v", n, err)
	}
}
