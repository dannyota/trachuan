package oidctest

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

func TestProvider_AuthorizationRequiresPKCE(t *testing.T) {
	p := New(t)
	p.EnableLogin()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	verifier := strings.Repeat("a", 43)
	challenge := oauth2.S256ChallengeFromVerifier(verifier)
	authorize := func(method, value string) *http.Response {
		t.Helper()
		query := url.Values{"redirect_uri": {"https://example.invalid/callback"}, "code_challenge_method": {method}, "code_challenge": {value}}
		resp, err := client.Get(p.URL + "/authorize?" + query.Encode())
		if err != nil {
			t.Fatal("authorization request failed")
		}
		return resp
	}
	for _, tc := range []struct{ name, method, challenge string }{
		{"missing challenge", "S256", ""}, {"missing method", "", challenge}, {"plain method", "plain", challenge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := authorize(tc.method, tc.challenge)
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusBadRequest || resp.Header.Get("Location") != "" {
				t.Fatalf("status %d", resp.StatusCode)
			}
		})
	}
	for _, tc := range []struct {
		name, verifier string
		status         int
	}{
		{"missing verifier", "", 400}, {"wrong verifier", strings.Repeat("b", 43), 400}, {"valid verifier", verifier, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := authorize("S256", challenge)
			location, err := resp.Location()
			_ = resp.Body.Close()
			if err != nil || resp.StatusCode != http.StatusFound || location.Query().Get("code") == "" {
				t.Fatal("authorization did not issue a code")
			}
			form := url.Values{"grant_type": {"authorization_code"}, "code": {location.Query().Get("code")}, "code_verifier": {tc.verifier}, "redirect_uri": {"https://example.invalid/callback"}}
			exchange, err := client.PostForm(p.URL+"/token", form)
			if err != nil {
				t.Fatal("token request failed")
			}
			defer func() { _ = exchange.Body.Close() }()
			if exchange.StatusCode != tc.status {
				t.Fatalf("status %d, want %d", exchange.StatusCode, tc.status)
			}
		})
	}
}
