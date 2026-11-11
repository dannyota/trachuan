package oauth

import (
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
				tokenJSON(t, requestToken(newTokenMemory().handler(), f), 400, "invalid_request")
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
