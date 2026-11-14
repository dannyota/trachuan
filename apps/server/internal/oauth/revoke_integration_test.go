//go:build integration

package oauth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/store"
)

func TestRevoke_PostgreSQLGrantAndCache(t *testing.T) {
	p := newTokenPG(t)
	first := p.exchange(t, "first-code")
	second := p.exchange(t, "second-code")
	p.code(t, "pending-code")
	v := &credential.Verifier{Store: store.New(p.pool), Audience: tokenTestBase + "/mcp"}
	for _, got := range []map[string]any{first, second} {
		if _, err := v.Verify(t.Context(), got["access_token"].(string)); err != nil {
			t.Fatal(err)
		}
	}
	h := NewRevokeHandler(p.pool, v.Evict, v.EvictGrant)
	tokenJSON(t, revokeCall(h, first["refresh_token"].(string), "other"), 400, "invalid_request")
	if _, err := v.Verify(t.Context(), first["access_token"].(string)); err != nil {
		t.Fatal("mismatch revoked access")
	}
	if w := revokeCall(h, first["refresh_token"].(string), "client"); w.Code != 200 {
		t.Fatal(w.Code)
	}
	p.assertRevoked(t)
	for _, got := range []map[string]any{first, second} {
		if _, err := v.Verify(t.Context(), got["access_token"].(string)); !errors.Is(err, credential.ErrUnauthorized) {
			t.Fatal("revoked grant cached")
		}
	}
}
func TestRevoke_PostgreSQLAccessOnly(t *testing.T) {
	p := newTokenPG(t)
	got := p.exchange(t, "first-code")
	v := &credential.Verifier{Store: store.New(p.pool), Audience: tokenTestBase + "/mcp"}
	raw := got["access_token"].(string)
	if _, err := v.Verify(t.Context(), raw); err != nil {
		t.Fatal(err)
	}
	h := NewRevokeHandler(p.pool, v.Evict, v.EvictGrant)
	if w := revokeCall(h, raw, "client"); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if _, err := v.Verify(t.Context(), raw); !errors.Is(err, credential.ErrUnauthorized) {
		t.Fatal("access cached")
	}
	tokenJSON(t, requestToken(p.handler, refreshForm(got["refresh_token"].(string))), 200, "")
}
func TestBearer_PostgreSQLAccountLifecycle(t *testing.T) {
	for _, change := range []string{"block", "deletion", "epoch", "resource"} {
		t.Run(change, func(t *testing.T) {
			p := newTokenPG(t)
			got := p.exchange(t, "first-code")
			q := store.New(p.pool)
			now := time.Now()
			v := &credential.Verifier{Store: q, Audience: tokenTestBase + "/mcp", Clock: func() time.Time { return now }}
			raw := got["access_token"].(string)
			if _, err := v.Verify(t.Context(), raw); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "block":
				if _, err := q.BlockAccount(t.Context(), p.account.ID); err != nil {
					t.Fatal(err)
				}
				v.EvictAccount(p.account.ID)
			case "deletion":
				session, sessionErr := q.CreateSession(t.Context(), store.CreateSessionParams{AccountID: p.account.ID, AuthEpoch: p.account.AuthEpoch, TokenHash: strings.Repeat("f", 64), CsrfSecret: "synthetic", ExpiresAt: time.Now().Add(time.Hour)})
				if sessionErr != nil {
					t.Fatal(sessionErr)
				}
				if _, err := q.RequestDeletion(t.Context(), store.RequestDeletionParams{AccountID: p.account.ID, SessionID: session.ID, AuthEpoch: session.AuthEpoch}); err != nil {
					t.Fatal(err)
				}
				v.EvictAccount(p.account.ID)
			case "epoch":
				if _, err := p.pool.Exec(t.Context(), `UPDATE account SET auth_epoch=auth_epoch+1 WHERE id=$1`, p.account.ID); err != nil {
					t.Fatal(err)
				}
				now = now.Add(10 * time.Second)
			case "resource":
				if _, err := p.pool.Exec(t.Context(), `UPDATE oauth_token SET resource='https://other.invalid/mcp' WHERE token_hash=$1`, tokenDigest(raw)); err != nil {
					t.Fatal(err)
				}
				now = now.Add(10 * time.Second)
			}
			if _, err := v.Verify(t.Context(), raw); !errors.Is(err, credential.ErrUnauthorized) {
				t.Fatal("changed authority accepted")
			}
		})
	}
}

func TestBearer_PostgreSQLGrantActivityThrottle(t *testing.T) {
	p := newTokenPG(t)
	got := p.exchange(t, "first-code")
	if _, err := p.pool.Exec(t.Context(), `UPDATE oauth_grant SET last_used_at=now()-interval '2 hours' WHERE id=$1`, p.grant.ID); err != nil {
		t.Fatal(err)
	}
	q := store.New(p.pool)
	v := &credential.Verifier{Store: q, Audience: tokenTestBase + "/mcp"}
	if _, err := v.Verify(t.Context(), got["access_token"].(string)); err != nil {
		t.Fatal(err)
	}
	rows, err := q.ListGrants(t.Context(), p.account.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("read grant activity: %v", err)
	}
	first := rows[0].LastUsedAt
	if first.Before(time.Now().Add(-time.Minute)) {
		t.Fatal("bearer use did not update grant activity")
	}
	if err = q.TouchOAuthGrant(t.Context(), p.grant.ID); err != nil {
		t.Fatal(err)
	}
	rows, err = q.ListGrants(t.Context(), p.account.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("read throttled grant activity: %v", err)
	}
	if !rows[0].LastUsedAt.Equal(first) {
		t.Fatal("grant activity written twice within the hour")
	}
}
