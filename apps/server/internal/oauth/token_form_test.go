package oauth

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTokenForm_UnknownIgnoredRepeatRejected(t *testing.T) {
	for _, tc := range []struct {
		name, body, kind, method, code string
		status                         int
	}{
		{"unknown ignored", codeForm().Encode() + "&unknown=yes", "application/x-www-form-urlencoded", "POST", "", 200},
		{"repeat known", codeForm().Encode() + "&client_id=client", "application/x-www-form-urlencoded", "POST", "invalid_request", 400},
		{"repeat unknown", codeForm().Encode() + "&extra=1&extra=2", "application/x-www-form-urlencoded", "POST", "invalid_request", 400},
		{"encoded duplicate", codeForm().Encode() + "&%63lient_id=client", "application/x-www-form-urlencoded", "POST", "invalid_request", 400},
		{"bad escape", codeForm().Encode() + "&extra=%ZZ", "application/x-www-form-urlencoded", "POST", "invalid_request", 400},
		{"oversize", codeForm().Encode() + "&extra=" + strings.Repeat("x", 4096), "application/x-www-form-urlencoded", "POST", "invalid_request", 400},
		{"wrong media", codeForm().Encode(), "application/json", "POST", "invalid_request", 400},
		{"get", codeForm().Encode(), "application/x-www-form-urlencoded", "GET", "invalid_request", 405},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTokenMemory()
			r := httptest.NewRequest(tc.method, "/oauth/token?client_id=attacker", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", tc.kind)
			w := httptest.NewRecorder()
			m.handler().ServeHTTP(w, r)
			tokenJSON(t, w, tc.status, tc.code)
		})
	}
}

func TestTokenForm_RequiredAndGrantErrors(t *testing.T) {
	for _, grant := range []string{"authorization_code", "refresh_token"} {
		form := codeForm()
		if grant == "refresh_token" {
			form = refreshForm("trr_fixture")
		}
		for _, field := range []string{"grant_type", "client_id", "resource", "code", "code_verifier", "refresh_token"} {
			if _, ok := form[field]; !ok {
				continue
			}
			t.Run(grant+" missing "+field, func(t *testing.T) {
				f := form.Clone()
				f.Del(field)
				want := "invalid_request"
				if field == "resource" {
					want = "invalid_target"
				}
				tokenJSON(t, requestToken(newTokenMemory().handler(), f), 400, want)
			})
		}
	}
	for _, tc := range []struct{ field, value, code string }{
		{"grant_type", "password", "unsupported_grant_type"}, {"resource", "https://wrong.invalid/mcp", "invalid_target"}, {"client_id", "missing", "invalid_client"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			form := codeForm()
			form.Set(tc.field, tc.value)
			tokenJSON(t, requestToken(newTokenMemory().handler(), form), 400, tc.code)
		})
	}
}

func TestTokenForm_BodyBoundaryAndHeaders(t *testing.T) {
	body := codeForm().Encode() + "&unknown="
	body += strings.Repeat("x", 4096-len(body))
	m := newTokenMemory()
	r := httptest.NewRequest("POST", "/oauth/token", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	w := httptest.NewRecorder()
	m.handler().ServeHTTP(w, r)
	tokenJSON(t, w, 200, "")
	r = httptest.NewRequest("POST", "/oauth/token", strings.NewReader(codeForm().Encode()))
	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	newTokenMemory().handler().ServeHTTP(w, r)
	tokenJSON(t, w, 400, "invalid_request")
	r = httptest.NewRequest("POST", "/oauth/token?"+codeForm().Encode(), strings.NewReader(""))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	newTokenMemory().handler().ServeHTTP(w, r)
	tokenJSON(t, w, 400, "invalid_request")
}

func TestTokenForm_ResourceMissingOrEmpty(t *testing.T) {
	for _, grant := range []string{"authorization_code", "refresh_token"} {
		for _, missing := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s missing=%t", grant, missing), func(t *testing.T) {
				form := codeForm()
				if grant == "refresh_token" {
					form = refreshForm("trr_fixture")
				}
				if missing {
					form.Del("resource")
				} else {
					form.Set("resource", "")
				}
				tokenJSON(t, requestToken(newTokenMemory().handler(), form), 400, "invalid_target")
			})
		}
	}
}

func TestToken_RejectsClientAuthentication(t *testing.T) {
	for _, grant := range []string{"authorization_code", "refresh_token"} {
		for _, auth := range []string{"basic", "bearer", "empty header", "client_secret", "empty client_secret", "repeated client_secret"} {
			t.Run(grant+" "+auth, func(t *testing.T) {
				m := newTokenMemory()
				form := codeForm()
				var raw string
				if grant == "refresh_token" {
					raw = seedRefresh(t, m)
					form = refreshForm(raw)
				}
				if strings.Contains(auth, "client_secret") {
					form.Set("client_secret", "")
					if auth == "client_secret" {
						form.Set("client_secret", "example")
					}
					if auth == "repeated client_secret" {
						form.Add("client_secret", "second")
					}
				}
				r := httptest.NewRequest("POST", "/oauth/token", strings.NewReader(form.Encode()))
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				switch auth {
				case "basic":
					r.Header.Set("Authorization", "Basic Zm9vOmJhcg==")
				case "bearer":
					r.Header.Set("Authorization", "Bearer example")
				case "empty header":
					r.Header.Set("Authorization", "")
				}
				w := httptest.NewRecorder()
				m.handler().ServeHTTP(w, r)
				tokenJSON(t, w, 401, "invalid_client")
				if w.Header().Get("WWW-Authenticate") != `Basic realm="trachuan"` {
					t.Fatal("missing basic challenge")
				}
				if grant == "authorization_code" && (m.code.ConsumedAt != nil || len(m.tokens) != 0) {
					t.Fatal("code consumed")
				}
				if grant == "refresh_token" && (m.tokens[tokenDigest(raw)].SupersededAt != nil || m.revoked) {
					t.Fatal("refresh consumed or grant revoked")
				}
			})
		}
	}
}

func TestTokenForm_RejectsClientAuthenticationParameters(t *testing.T) {
	for _, grant := range []string{"authorization_code", "refresh_token"} {
		for _, parameter := range []string{"client_secret", "client_assertion", "client_assertion_type"} {
			for _, values := range [][]string{{"example"}, {""}, {"first", "second"}} {
				t.Run(fmt.Sprintf("%s/%s/%q", grant, parameter, values), func(t *testing.T) {
					m := newTokenMemory()
					form := codeForm()
					var raw string
					if grant == "refresh_token" {
						raw = seedRefresh(t, m)
						form = refreshForm(raw)
					}
					form[parameter] = values
					commits, rollbacks := m.commits, m.rollbacks
					w := requestToken(m.handler(), form)
					if (grant == "authorization_code" && m.code.ConsumedAt != nil) || m.revoked || m.tokens[tokenDigest(raw)].SupersededAt != nil {
						t.Error("authentication attempt consumed a credential or revoked the grant")
					}
					if m.commits != commits || m.rollbacks != rollbacks {
						t.Error("authentication attempt opened a token transaction")
					}
					if grant == "authorization_code" && len(m.tokens) != 0 {
						t.Error("authentication attempt issued tokens")
					}
					tokenJSON(t, w, 401, "invalid_client")
					if w.Header().Get("WWW-Authenticate") != `Basic realm="trachuan"` {
						t.Fatal("missing basic challenge")
					}
				})
			}
		}
	}
}
