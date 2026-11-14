package oauth

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func seedRefresh(t *testing.T, m *tokenMemory) string {
	t.Helper()
	got := tokenJSON(t, requestToken(m.handler(), codeForm()), 200, "")
	return got["refresh_token"].(string)
}
func TestRefresh_RotatesAndKeepsFamilyLife(t *testing.T) {
	m := newTokenMemory()
	raw := seedRefresh(t, m)
	before := m.tokens[tokenDigest(raw)]
	got := tokenJSON(t, requestToken(m.handler(), refreshForm(raw)), 200, "")
	assertTokenPair(t, m, got)
	after := m.tokens[tokenDigest(got["refresh_token"].(string))]
	if after.FamilyID != before.FamilyID || !after.FamilyExpiresAt.Equal(before.FamilyExpiresAt) || !after.ExpiresAt.Equal(before.FamilyExpiresAt) || after.RotatedFrom == nil || *after.RotatedFrom != before.TokenHash || m.tokens[before.TokenHash].SupersededAt == nil {
		t.Fatal("rotation lost lineage or extended family")
	}
	for _, tc := range []struct {
		name   string
		mutate func(*tokenMemory)
	}{
		{"epoch", func(m *tokenMemory) { m.account.AuthEpoch++ }},
		{"blocked", func(m *tokenMemory) { now := time.Now(); m.account.BlockedAt = &now }},
		{"deletion", func(m *tokenMemory) { now := time.Now(); m.account.DeleteRequestedAt = &now }},
		{"expiry", func(m *tokenMemory) {
			v := m.tokens[tokenDigest(raw)]
			v.ExpiresAt = time.Now().Add(-time.Second)
			m.tokens[v.TokenHash] = v
		}},
		{"family expiry", func(m *tokenMemory) {
			v := m.tokens[tokenDigest(raw)]
			v.FamilyExpiresAt = time.Now().Add(-time.Second)
			m.tokens[v.TokenHash] = v
		}},
		{"resource", func(m *tokenMemory) {
			v := m.tokens[tokenDigest(raw)]
			v.Resource = "https://other.invalid/mcp"
			m.tokens[v.TokenHash] = v
		}},
		{"kind", func(m *tokenMemory) { v := m.tokens[tokenDigest(raw)]; v.Kind = "access"; m.tokens[v.TokenHash] = v }},
		{"revoked token", func(m *tokenMemory) {
			v := m.tokens[tokenDigest(raw)]
			now := time.Now()
			v.RevokedAt = &now
			m.tokens[v.TokenHash] = v
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fresh := newTokenMemory()
			raw = seedRefresh(t, fresh)
			tc.mutate(fresh)
			tokenJSON(t, requestToken(fresh.handler(), refreshForm(raw)), 400, "invalid_grant")
			if fresh.tokens[tokenDigest(raw)].SupersededAt != nil {
				t.Fatal("invalid refresh consumed token")
			}
		})
	}
}
func TestRefresh_ScopeOptional(t *testing.T) {
	for _, scope := range []string{"absent", "mcp:read", "", "other", "mcp:read other"} {
		t.Run(scope, func(t *testing.T) {
			m := newTokenMemory()
			raw := seedRefresh(t, m)
			f := refreshForm(raw)
			if scope != "absent" {
				f.Set("scope", scope)
			}
			if scope == "absent" || scope == "mcp:read" {
				tokenJSON(t, requestToken(m.handler(), f), 200, "")
			} else {
				tokenJSON(t, requestToken(m.handler(), f), 400, "invalid_scope")
				if m.tokens[tokenDigest(raw)].SupersededAt != nil {
					t.Fatal("invalid scope rotated token")
				}
			}
		})
	}
}
func TestRefresh_ConcurrentReuseRevokesGrant(t *testing.T) {
	m := newTokenMemory()
	raw := seedRefresh(t, m)
	h := m.handler()
	m.code.ConsumedAt = nil
	seedRefresh(t, m)
	m.code.ConsumedAt = nil
	results := make(chan int, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() { results <- requestToken(h, refreshForm(raw)).Code })
	}
	wg.Wait()
	close(results)
	success, invalid := 0, 0
	for status := range results {
		switch status {
		case 200:
			success++
		case 400:
			invalid++
		default:
			t.Fatalf("status %d", status)
		}
	}
	if success != 1 || invalid != 1 || !m.revoked || m.code.RevokedAt == nil || m.invalidations != 1 {
		t.Fatal("reuse did not revoke entire grant")
	}
	for _, v := range m.tokens {
		if v.RevokedAt == nil {
			t.Fatal("reuse left a live token")
		}
	}
}
func TestToken_DatabaseErrors503AndUnknownCommit(t *testing.T) {
	for _, grant := range []string{"code", "refresh"} {
		stages := []string{"begin", "pg_advisory_xact_lock_shared", "name: GetClient", "name: TouchClient", "name: LockCredentialAccount", "name: LockGrantAuthority", "name: CreateTokenFamily", "commit"}
		if grant == "code" {
			stages = append(stages, "name: GetCodeIdentity", "name: ConsumeCode", "name: PruneTokenFamilies")
		} else {
			stages = append(stages, "name: GetToken", "name: RotateRefresh")
		}
		for _, stage := range stages {
			t.Run(grant+" "+stage, func(t *testing.T) {
				m := newTokenMemory()
				form := codeForm()
				if grant == "refresh" {
					form = refreshForm(seedRefresh(t, m))
				}
				switch stage {
				case "begin":
					m.beginErr = errors.New("begin failed")
				case "commit":
					m.commitErr = errors.New("connection lost after commit")
				default:
					m.fail = stage
				}
				before := len(m.tokens)
				tokenJSON(t, requestToken(m.handler(), form), 503, "temporarily_unavailable")
				if stage != "commit" && len(m.tokens) != before {
					t.Fatal("failed transaction changed credentials")
				}
				if stage == "commit" {
					m.commitErr = nil
					tokenJSON(t, requestToken(m.handler(), form), 400, "invalid_grant")
					if !m.revoked {
						t.Fatal("unknown commit retry did not fail closed")
					}
				}
			})
		}
	}
}

func TestRefresh_RejectsForeignAndUnknownCredentials(t *testing.T) {
	for _, field := range []string{"client_id", "resource", "refresh_token"} {
		t.Run(field, func(t *testing.T) {
			m := newTokenMemory()
			raw := seedRefresh(t, m)
			form := refreshForm(raw)
			value, code := "unknown", "invalid_grant"
			switch field {
			case "client_id":
				value = "other"
			case "resource":
				value = "https://other.invalid/mcp"
				code = "invalid_target"
			}
			form.Set(field, value)
			tokenJSON(t, requestToken(m.handler(), form), 400, code)
			if m.revoked || m.tokens[tokenDigest(raw)].SupersededAt != nil {
				t.Fatal("foreign request changed credentials")
			}
		})
	}
}

func TestRefresh_AccessExpiryClampedToFamily(t *testing.T) {
	m := newTokenMemory()
	raw := seedRefresh(t, m)
	old := m.tokens[tokenDigest(raw)]
	old.FamilyExpiresAt = time.Now().Add(30 * time.Minute)
	old.ExpiresAt = old.FamilyExpiresAt
	m.tokens[old.TokenHash] = old
	got := tokenJSON(t, requestToken(m.handler(), refreshForm(raw)), 200, "")
	access := m.tokens[tokenDigest(got["access_token"].(string))]
	expires := got["expires_in"].(float64)
	if !access.ExpiresAt.Equal(old.FamilyExpiresAt) || expires < 1799 || expires > 1800 {
		t.Fatal("access token outlives family")
	}
}

func TestToken_ReplayRevocationFailures(t *testing.T) {
	for _, grant := range []string{"code", "refresh"} {
		for _, stage := range []string{"revoke", "commit"} {
			t.Run(grant+" "+stage, func(t *testing.T) {
				m := newTokenMemory()
				raw := seedRefresh(t, m)
				form := codeForm()
				if grant == "refresh" {
					tokenJSON(t, requestToken(m.handler(), refreshForm(raw)), 200, "")
					form = refreshForm(raw)
				}
				if stage == "revoke" {
					m.fail = "name: RevokeGrant"
				} else {
					m.commitErr = errors.New("unknown commit")
				}
				tokenJSON(t, requestToken(m.handler(), form), 503, "temporarily_unavailable")
				if stage == "revoke" && (m.revoked || m.invalidations != 0) {
					t.Fatal("rolled back revocation changed grant")
				}
				if stage == "commit" && (!m.revoked || m.invalidations != 1) {
					t.Fatal("unknown revocation commit kept cached credentials")
				}
			})
		}
	}
}

func TestRefresh_ClientWithoutGrantRejected(t *testing.T) {
	m := newTokenMemory()
	raw := seedRefresh(t, m)
	m.grants = []string{"authorization_code"}
	tokenJSON(t, requestToken(m.handler(), refreshForm(raw)), 400, "unauthorized_client")
	if m.tokens[tokenDigest(raw)].SupersededAt != nil || m.revoked {
		t.Fatal("unauthorized client consumed refresh token")
	}
}
