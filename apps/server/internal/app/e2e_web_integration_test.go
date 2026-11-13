//go:build integration

package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
)

type e2eToken struct {
	ID    int64  `json:"id"`
	Token string `json:"token"`
}

func (f *e2eFixture) personal(c *http.Client, csrf string) e2eToken {
	f.t.Helper()
	var token e2eToken
	f.json(f.request(c, "POST", "/api/v1/me/tokens", `{"name":"fixture"}`, "application/json", csrf, ""), 201, &token)
	if token.ID <= 0 || token.Token == "" {
		f.t.Fatal("invalid personal token shape")
	}
	return token
}
func TestE2E_WebLoginTokensAndLogout(t *testing.T) {
	f := newE2E(t)
	first, second := f.client(), f.client()
	f.login(first)
	me := f.me(first)
	base, err := url.Parse(f.server.URL)
	if err != nil {
		t.Fatal("invalid server URL")
	}
	var session *http.Cookie
	for _, cookie := range first.Jar.Cookies(base) {
		if cookie.Name == "__Host-session" {
			session = cookie
		}
	}
	if session == nil {
		t.Fatal("missing session cookie")
	}
	for _, tc := range []struct{ name, origin, csrf string }{
		{"missing csrf", f.server.URL, ""}, {"wrong csrf", f.server.URL, "wrong"},
		{"missing origin", "", me.CSRF}, {"wrong origin", "https://other.invalid", me.CSRF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f.status(f.postWithHeaders(first, "/api/v1/me/tokens", `{"name":"fixture"}`, "application/json", tc.origin, tc.csrf), 403)
		})
	}
	var tokens []json.RawMessage
	f.json(f.request(first, "GET", "/api/v1/me/tokens", "", "", "", ""), 200, &tokens)
	if len(tokens) != 0 {
		t.Fatal("rejected mutation created a token")
	}
	token := f.personal(first, me.CSRF)
	f.mcp(first, token.Token, 200)
	f.status(f.request(first, "DELETE", fmt.Sprintf("/api/v1/me/tokens/%d", token.ID), "", "", me.CSRF, ""), 204)
	f.mcp(first, token.Token, 401)
	f.login(second)
	other := f.me(second)
	if me.ID != other.ID {
		t.Fatal("login changed account")
	}
	r := f.request(first, "POST", "/api/v1/auth/logout", "", "", me.CSRF, "")
	cleared := false
	for _, ck := range r.Cookies() {
		if ck.Name == "__Host-session" && ck.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("session cookie not cleared")
	}
	f.status(r, 204)
	f.status(f.request(first, "GET", "/api/v1/me", "", "", "", ""), 204)
	replay := f.client()
	replay.Jar.SetCookies(base, []*http.Cookie{session})
	f.status(f.request(replay, "GET", "/api/v1/me", "", "", "", ""), 401)
	f.me(second)
}
