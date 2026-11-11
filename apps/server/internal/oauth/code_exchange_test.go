package oauth

import (
	"strings"
	"testing"
	"time"
)

func TestCode_ExchangeBindings(t *testing.T) {
	for _, tc := range []struct {
		name, field, value string
		mutate             func(*tokenMemory)
		want               int
	}{
		{name: "redirect absent", want: 200},
		{name: "redirect exact", field: "redirect_uri", value: "http://localhost/cb", want: 200},
		{name: "redirect mismatch", field: "redirect_uri", value: "http://localhost:8080/cb", want: 400},
		{name: "redirect empty", field: "redirect_uri", value: "", want: 400},
		{name: "client mismatch", field: "client_id", value: "other", want: 400},
		{name: "pkce mismatch", field: "code_verifier", value: strings.Repeat("w", 43), want: 400},
		{name: "pkce malformed", field: "code_verifier", value: "short", want: 400},
		{name: "unknown code", field: "code", value: "unknown", want: 400},
		{name: "stored resource", mutate: func(m *tokenMemory) { m.code.Resource = "https://wrong.invalid/mcp" }, want: 400},
		{name: "stored scope", mutate: func(m *tokenMemory) { m.code.Scope = "other" }, want: 400},
		{name: "epoch", mutate: func(m *tokenMemory) { m.account.AuthEpoch++ }, want: 400},
		{name: "blocked", mutate: func(m *tokenMemory) { now := time.Now(); m.account.BlockedAt = &now }, want: 400},
		{name: "deletion", mutate: func(m *tokenMemory) { now := time.Now(); m.account.DeleteRequestedAt = &now }, want: 400},
		{name: "expiry", mutate: func(m *tokenMemory) { m.code.ExpiresAt = time.Now().Add(-time.Second) }, want: 400},
		{name: "revoked grant", mutate: func(m *tokenMemory) { m.revoked = true }, want: 400},
		{name: "revoked code", mutate: func(m *tokenMemory) { now := time.Now(); m.code.RevokedAt = &now }, want: 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTokenMemory()
			if tc.mutate != nil {
				tc.mutate(m)
			}
			form := codeForm()
			if tc.field != "" {
				form.Set(tc.field, tc.value)
			}
			code := ""
			if tc.want == 400 {
				code = "invalid_grant"
			}
			got := tokenJSON(t, requestToken(m.handler(), form), tc.want, code)
			if tc.want == 200 {
				assertTokenPair(t, m, got)
			} else if m.code.ConsumedAt != nil || len(m.tokens) != 0 {
				t.Fatal("rejected exchange changed credentials")
			}
		})
	}
}

func assertTokenPair(t *testing.T, m *tokenMemory, got map[string]any) {
	t.Helper()
	if got["token_type"] != "Bearer" || got["scope"] != "mcp:read" || got["expires_in"] != float64(3600) {
		t.Fatal("token response metadata mismatch")
	}
	for field, prefix := range map[string]string{"access_token": "tra_", "refresh_token": "trr_"} {
		raw, ok := got[field].(string)
		if !ok || !strings.HasPrefix(raw, prefix) || len(raw) != 47 {
			t.Fatalf("invalid %s format", field)
		}
		row, ok := m.tokens[tokenDigest(raw)]
		if !ok || row.AuthEpoch != 7 || row.Resource != tokenTestBase+"/mcp" || row.Scope != "mcp:read" {
			t.Fatal("missing or incorrect stored token")
		}
		if _, ok := m.tokens[raw]; ok {
			t.Fatal("raw token stored")
		}
	}
}

func TestCode_ReplayRevokesGrant(t *testing.T) {
	m := newTokenMemory()
	h := m.handler()
	first := tokenJSON(t, requestToken(h, codeForm()), 200, "")
	assertTokenPair(t, m, first)
	// Wrong bindings must not let another client revoke this grant.
	wrong := codeForm()
	wrong.Set("client_id", "other")
	tokenJSON(t, requestToken(h, wrong), 400, "invalid_grant")
	if m.revoked {
		t.Fatal("foreign client revoked grant")
	}
	m.code.ExpiresAt = time.Now().Add(-time.Minute)
	tokenJSON(t, requestToken(h, codeForm()), 400, "invalid_grant")
	if !m.revoked || m.invalidations != 1 {
		t.Fatal("replay did not revoke and invalidate grant")
	}
	for _, token := range m.tokens {
		if token.RevokedAt == nil {
			t.Fatal("replay left token live")
		}
	}
}

func TestFamilies_CapRevokesOldest(t *testing.T) {
	m := newTokenMemory()
	h := m.handler()
	oldest := ""
	for i := range 11 {
		m.code.CodeHash = tokenDigest("fixture-code")
		m.code.ConsumedAt = nil
		got := tokenJSON(t, requestToken(h, codeForm()), 200, "")
		if i == 0 {
			oldest = m.tokens[tokenDigest(got["refresh_token"].(string))].FamilyID
		}
	}
	live := map[string]bool{}
	for _, v := range m.tokens {
		if v.FamilyID == oldest && v.RevokedAt == nil {
			t.Fatal("oldest family survived cap")
		}
		if v.RevokedAt == nil {
			live[v.FamilyID] = true
		}
	}
	if len(live) != 10 || m.revoked {
		t.Fatalf("live families %d, grant revoked %v", len(live), m.revoked)
	}
}

func TestFamilies_EvictsCachedCredentials(t *testing.T) {
	m := newTokenMemory()
	h := m.handler()
	for range 11 {
		m.code.ConsumedAt = nil
		tokenJSON(t, requestToken(h, codeForm()), 200, "")
	}
	if m.invalidations != 1 {
		t.Fatalf("cache invalidations %d, want 1", m.invalidations)
	}
}

func TestCode_ClientWithoutGrantRejected(t *testing.T) {
	m := newTokenMemory()
	m.grants = []string{"refresh_token"}
	tokenJSON(t, requestToken(m.handler(), codeForm()), 400, "unauthorized_client")
	if m.code.ConsumedAt != nil || len(m.tokens) != 0 {
		t.Fatal("unauthorized client consumed code")
	}
}

func TestCode_ClientWithoutRefreshGetsAccessOnly(t *testing.T) {
	m := newTokenMemory()
	m.grants = []string{"authorization_code"}
	before := time.Now()
	got := tokenJSON(t, requestToken(m.handler(), codeForm()), 200, "")
	after := time.Now()
	if _, ok := got["refresh_token"]; ok {
		t.Fatal("refresh token returned to client without refresh grant")
	}
	access, ok := got["access_token"].(string)
	if !ok || !strings.HasPrefix(access, "tra_") || len(access) != 47 {
		t.Fatal("invalid access token")
	}
	if got["expires_in"] != float64(3600) || got["token_type"] != "Bearer" || got["scope"] != "mcp:read" {
		t.Fatal("invalid access response")
	}
	if len(m.tokens) != 1 {
		t.Fatalf("got %d tokens, want one", len(m.tokens))
	}
	row, ok := m.tokens[tokenDigest(access)]
	if !ok || row.Kind != "access" || !row.ExpiresAt.Equal(row.FamilyExpiresAt) || row.ExpiresAt.Before(before.Add(time.Hour)) || row.ExpiresAt.After(after.Add(time.Hour)) {
		t.Fatal("access-only family has wrong lifetime")
	}
}
