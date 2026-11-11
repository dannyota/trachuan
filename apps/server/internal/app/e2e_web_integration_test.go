//go:build integration

package app

import (
	"fmt"
	"net/http"
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
	f.status(f.request(first, "GET", "/api/v1/me", "", "", "", ""), 401)
	f.me(second)
}
