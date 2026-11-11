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

type oauthRaceResult struct {
	marker bool
	err    error
}

func oauthRace(t *testing.T, ctx context.Context, conn *pgx.Conn, a, b func(*store.Queries) oauthRaceResult) [2]oauthRaceResult {
	t.Helper()
	other, err := pgx.ConnectConfig(ctx, conn.Config().Copy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = other.Close(ctx) })
	start := make(chan struct{})
	results := make(chan struct {
		i      int
		result oauthRaceResult
	}, 2)
	for i, f := range []func(*store.Queries) oauthRaceResult{a, b} {
		q := store.New([]*pgx.Conn{conn, other}[i])
		go func(i int, f func(*store.Queries) oauthRaceResult) {
			<-start
			results <- struct {
				i      int
				result oauthRaceResult
			}{i, f(q)}
		}(i, f)
	}
	close(start)
	var got [2]oauthRaceResult
	for range 2 {
		r := <-results
		got[r.i] = r.result
	}
	return got
}

func TestStore_ConcurrentOAuthClaims(t *testing.T) {
	ctx, conn, q, account, _ := oauthFixture(t)
	handle, browser := strings.Repeat("a", 64), strings.Repeat("b", 64)
	_, err := q.CreateRequest(ctx, store.CreateRequestParams{HandleHash: handle, BrowserHash: browser, ClientID: "client", RedirectUri: "https://example.invalid/cb", State: "state", CodeChallenge: "challenge", Resource: "https://example.invalid/mcp", Scope: "mcp:read", ExpiresAt: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	claim := func(q *store.Queries) oauthRaceResult {
		_, err := q.ClaimRequest(ctx, store.ClaimRequestParams{HandleHash: handle, BrowserHash: browser, AccountID: &account})
		return oauthRaceResult{err: err}
	}
	got := oauthRace(t, ctx, conn, claim, claim)
	wins, misses := 0, 0
	for _, r := range got {
		if r.err == nil {
			wins++
		} else if errors.Is(r.err, pgx.ErrNoRows) {
			misses++
		} else {
			t.Fatal(r.err)
		}
	}
	if wins != 1 || misses != 1 {
		t.Fatalf("wins=%d misses=%d", wins, misses)
	}
}

func TestStore_ConcurrentOAuthCodeConsumption(t *testing.T) {
	ctx, conn, q, _, grant := oauthFixture(t)
	hash := strings.Repeat("a", 64)
	oauthCode(t, ctx, q, grant, hash, time.Now().Add(time.Minute))
	consume := func(q *store.Queries) oauthRaceResult {
		row, err := q.ConsumeCode(ctx, hash)
		return oauthRaceResult{marker: row.Replay == true, err: err}
	}
	got := oauthRace(t, ctx, conn, consume, consume)
	if got[0].err != nil || got[1].err != nil || got[0].marker == got[1].marker {
		t.Fatalf("results=%+v", got)
	}
}

func TestStore_ConcurrentOAuthRefresh(t *testing.T) {
	ctx, conn, q, _, grant := oauthFixture(t)
	hash := strings.Repeat("a", 64)
	oauthToken(t, ctx, q, grant, hash, time.Now().Add(time.Hour))
	rotate := func(q *store.Queries) oauthRaceResult {
		row, err := q.RotateRefresh(ctx, hash)
		return oauthRaceResult{marker: row.Reuse == true, err: err}
	}
	got := oauthRace(t, ctx, conn, rotate, rotate)
	if got[0].err != nil || got[1].err != nil || got[0].marker == got[1].marker {
		t.Fatalf("results=%+v", got)
	}
}

func TestStore_RevokeRacesRefresh(t *testing.T) {
	ctx, conn, q, _, grant := oauthFixture(t)
	hash := strings.Repeat("a", 64)
	oauthToken(t, ctx, q, grant, hash, time.Now().Add(time.Hour))
	rotate := func(q *store.Queries) oauthRaceResult {
		row, err := q.RotateRefresh(ctx, hash)
		return oauthRaceResult{marker: row.Reuse == true, err: err}
	}
	revoke := func(q *store.Queries) oauthRaceResult {
		_, err := q.RevokeGrant(ctx, grant)
		return oauthRaceResult{err: err}
	}
	got := oauthRace(t, ctx, conn, rotate, revoke)
	if got[1].err != nil || (got[0].err != nil && !errors.Is(got[0].err, pgx.ErrNoRows)) || got[0].marker {
		t.Fatalf("results=%+v", got)
	}
	row, err := q.GetToken(ctx, hash)
	if err != nil || row.RevokedAt == nil || row.GrantRevokedAt == nil {
		t.Fatalf("token not revoked: err=%v", err)
	}
}

func TestStore_IssueRacesGrantCleanup(t *testing.T) {
	ctx, conn, q, _, grant := oauthFixture(t)
	now := time.Now().UTC()
	if _, err := conn.Exec(ctx, `UPDATE oauth_grant SET retirement_anchor = $2 WHERE id = $1`, grant, now.Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	hash := strings.Repeat("a", 64)
	oauthCode(t, ctx, store.New(tx), grant, hash, now.Add(time.Hour))
	other, err := pgx.ConnectConfig(ctx, conn.Config().Copy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = other.Close(ctx) })
	period := pgtype.Interval{Microseconds: int64(24 * time.Hour / time.Microsecond), Valid: true}
	result := make(chan oauthRaceResult, 1)
	go func() {
		n, err := store.New(other).DeleteRetiredOAuthGrants(ctx, store.DeleteRetiredOAuthGrantsParams{AsOf: now, Retention: period, BatchLimit: 1})
		result <- oauthRaceResult{marker: n != 0, err: err}
	}()
	r := <-result
	if r.err != nil || r.marker {
		t.Fatalf("cleanup while issuing: %+v", r)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	n, err := q.DeleteRetiredOAuthGrants(ctx, store.DeleteRetiredOAuthGrantsParams{AsOf: now, Retention: period, BatchLimit: 1})
	if err != nil || n != 0 {
		t.Fatalf("cleanup after issuance: count=%d err=%v", n, err)
	}
	var exists bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM oauth_code WHERE code_hash = $1)`, hash).Scan(&exists); err != nil || !exists {
		t.Fatalf("code exists=%v err=%v", exists, err)
	}
}

func TestStore_ConcurrentIssueAndCleanup(t *testing.T) {
	ctx, conn, _, _, grant := oauthFixture(t)
	now := time.Now().UTC()
	if _, err := conn.Exec(ctx, `UPDATE oauth_grant SET retirement_anchor = $2 WHERE id = $1`, grant, now.Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	hash := strings.Repeat("a", 64)
	issue := func(q *store.Queries) oauthRaceResult {
		_, err := q.CreateCode(ctx, store.CreateCodeParams{GrantID: grant, CodeHash: hash, RedirectUri: "https://example.invalid/cb", Resource: "https://example.invalid/mcp", CodeChallenge: "challenge", Scope: "mcp:read", ExpiresAt: now.Add(time.Hour)})
		return oauthRaceResult{err: err}
	}
	cleanup := func(q *store.Queries) oauthRaceResult {
		n, err := q.DeleteRetiredOAuthGrants(ctx, store.DeleteRetiredOAuthGrantsParams{AsOf: now, Retention: pgtype.Interval{Microseconds: int64(24 * time.Hour / time.Microsecond), Valid: true}, BatchLimit: 1})
		return oauthRaceResult{marker: n == 1, err: err}
	}
	got := oauthRace(t, ctx, conn, issue, cleanup)
	if got[1].err != nil || (got[0].err != nil && !errors.Is(got[0].err, pgx.ErrNoRows)) {
		t.Fatalf("results=%+v", got)
	}
	if got[0].err == nil {
		var exists bool
		if err := conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM oauth_code WHERE code_hash = $1)`, hash).Scan(&exists); err != nil || !exists {
			t.Fatalf("issued code exists=%v cleanup=%+v err=%v", exists, got[1], err)
		}
	} else if !got[1].marker {
		t.Fatalf("issuance failed without deletion: %+v", got)
	}
}

func TestStore_OAuthExpiryBoundaries(t *testing.T) {
	ctx, conn, q, account, grant := oauthFixture(t)
	handle, browser, code := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)
	_, err := q.CreateRequest(ctx, store.CreateRequestParams{HandleHash: handle, BrowserHash: browser, ClientID: "client", RedirectUri: "https://example.invalid/cb", State: "state", CodeChallenge: "challenge", Resource: "https://example.invalid/mcp", Scope: "mcp:read", ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	oauthCode(t, ctx, q, grant, code, time.Now().Add(time.Hour))
	if _, err := conn.Exec(ctx, `UPDATE oauth_request SET expires_at = now() WHERE handle_hash = $1`, handle); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `UPDATE oauth_code SET expires_at = now() WHERE code_hash = $1`, code); err != nil {
		t.Fatal(err)
	}
	if _, err := q.ClaimRequest(ctx, store.ClaimRequestParams{HandleHash: handle, BrowserHash: browser, AccountID: &account}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expired claim: %v", err)
	}
	if _, err := q.ConsumeCode(ctx, code); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expired code: %v", err)
	}
}
