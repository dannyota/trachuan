//go:build integration

package app

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/oauth2"
)

type e2eOAuth struct{ client, access, refresh string }

func (f *e2eFixture) grant(c *http.Client, signedOut bool) e2eOAuth {
	f.t.Helper()
	const redirect = "http://127.0.0.1:12345/callback"
	var registration struct {
		Client    string   `json:"client_id"`
		Redirects []string `json:"redirect_uris"`
	}
	f.json(f.request(c, "POST", "/oauth/register", `{"client_name":"Fixture","redirect_uris":["http://127.0.0.1:12345/callback"],"grant_types":["authorization_code","refresh_token"],"response_types":["code"],"token_endpoint_auth_method":"none"}`, "application/json", "", ""), 201, &registration)
	if registration.Client == "" || len(registration.Redirects) != 1 || registration.Redirects[0] != redirect {
		f.t.Fatal("invalid registration shape")
	}
	verifier := strings.Repeat("a", 43)
	state := "fixture-state"
	v := url.Values{"client_id": {registration.Client}, "redirect_uri": {redirect}, "response_type": {"code"}, "code_challenge": {oauth2.S256ChallengeFromVerifier(verifier)}, "code_challenge_method": {"S256"}, "state": {state}, "resource": {f.server.URL + "/mcp"}, "scope": {"mcp:read"}}
	r := f.request(c, "GET", "/oauth/authorize?"+v.Encode(), "", "", "", "")
	path := r.Header.Get("Location")
	cookie := false
	for _, ck := range r.Cookies() {
		if ck.Name == "__Host-oauth" && ck.Secure && ck.HttpOnly && ck.SameSite == http.SameSiteLaxMode && ck.Path == "/" && ck.MaxAge == 600 {
			cookie = true
		}
	}
	if !cookie || !strings.HasPrefix(path, "/oauth/consent/") {
		f.t.Fatal("missing consent redirect or cookie")
	}
	f.status(r, 303)
	if signedOut {
		base, err := url.Parse(f.server.URL)
		if err != nil {
			f.t.Fatal("invalid server URL")
		}
		var oauthCookie string
		for _, ck := range c.Jar.Cookies(base) {
			if ck.Name == "__Host-oauth" {
				oauthCookie = ck.Value
			}
		}
		if oauthCookie == "" {
			f.t.Fatal("missing browser cookie")
		}
		r = f.request(c, "GET", path, "", "", "", "")
		loginURL, err := url.Parse(r.Header.Get("Location"))
		if err != nil || loginURL.Path != "/login" || loginURL.Query().Get("next") != path {
			f.t.Fatal("invalid login redirect")
		}
		f.status(r, 303)
		r = f.request(c, "GET", "/api/v1/auth/google/start?next="+url.QueryEscape(loginURL.Query().Get("next")), "", "", "", "")
		location := r.Header.Get("Location")
		f.status(r, 302)
		r = f.request(c, "GET", location, "", "", "", "")
		location = r.Header.Get("Location")
		f.status(r, 302)
		r = f.request(c, "GET", location, "", "", "", "")
		if r.Header.Get("Location") != path {
			f.t.Fatal("login did not return to consent")
		}
		f.status(r, 302)
		found := false
		for _, ck := range c.Jar.Cookies(base) {
			if ck.Name == "__Host-oauth" && ck.Value == oauthCookie {
				found = true
			}
		}
		if !found {
			f.t.Fatal("browser cookie did not survive login")
		}
	}
	r = f.request(c, "GET", path, "", "", "", "")
	if r.StatusCode != 200 {
		f.status(r, 200)
	}
	root, err := html.Parse(r.Body)
	discard(r)
	if err != nil {
		f.t.Fatal("invalid consent page")
	}
	var formToken string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "input" {
			attrs := map[string]string{}
			for _, a := range n.Attr {
				attrs[a.Key] = a.Val
			}
			if attrs["name"] == "csrf_token" {
				formToken = attrs["value"]
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	if formToken == "" {
		f.t.Fatal("missing consent form token")
	}
	form := url.Values{"csrf_token": {formToken}, "decision": {"approve"}}
	if signedOut {
		base, parseErr := url.Parse(f.server.URL)
		if parseErr != nil {
			f.t.Fatal("invalid server URL")
		}
		jar, jarErr := cookiejar.New(nil)
		if jarErr != nil {
			f.t.Fatal("cookie jar failed")
		}
		for _, ck := range c.Jar.Cookies(base) {
			if ck.Name == "__Host-session" {
				jar.SetCookies(base, []*http.Cookie{ck})
			}
		}
		withoutOAuth := *c
		withoutOAuth.Jar = jar
		for _, tc := range []struct {
			name, token, origin string
			client              *http.Client
		}{
			{"missing form token", "", f.server.URL, c},
			{"wrong form token", "wrong", f.server.URL, c},
			{"wrong origin", formToken, "https://other.invalid", c},
			{"missing browser cookie", formToken, f.server.URL, &withoutOAuth},
		} {
			f.t.Run(tc.name, func(t *testing.T) {
				attempt := url.Values{"csrf_token": {tc.token}, "decision": {"approve"}}
				f.status(f.postWithHeaders(tc.client, path, attempt.Encode(), "application/x-www-form-urlencoded", tc.origin, ""), 400)
				f.status(f.request(c, "GET", path, "", "", "", ""), 200)
			})
		}
		var grants []struct {
			ID int64 `json:"id"`
		}
		f.json(f.request(c, "GET", "/api/v1/me/grants", "", "", "", ""), 200, &grants)
		if len(grants) != 0 {
			f.t.Fatal("rejected consent created a grant")
		}
	}
	r = f.request(c, "POST", path, form.Encode(), "application/x-www-form-urlencoded", formToken, "")
	location := r.Header.Get("Location")
	f.status(r, 303)
	u, err := url.Parse(location)
	if err != nil || u.Scheme != "http" || u.Host != "127.0.0.1:12345" || u.Path != "/callback" || u.Query().Get("state") != state || u.Query().Get("iss") != f.server.URL || u.Query().Get("code") == "" {
		f.t.Fatal("invalid authorization redirect")
	}
	exchange := url.Values{"grant_type": {"authorization_code"}, "client_id": {registration.Client}, "redirect_uri": {redirect}, "code": {u.Query().Get("code")}, "code_verifier": {verifier}, "resource": {f.server.URL + "/mcp"}}
	var result struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		Type    string `json:"token_type"`
	}
	f.json(f.request(c, "POST", "/oauth/token", exchange.Encode(), "application/x-www-form-urlencoded", "", ""), 200, &result)
	if result.Access == "" || result.Refresh == "" || !strings.EqualFold(result.Type, "bearer") {
		f.t.Fatal("invalid token response shape")
	}
	return e2eOAuth{registration.Client, result.Access, result.Refresh}
}
func (f *e2eFixture) refresh(c *http.Client, grant e2eOAuth, want int) e2eOAuth {
	f.t.Helper()
	form := url.Values{"grant_type": {"refresh_token"}, "client_id": {grant.client}, "refresh_token": {grant.refresh}, "resource": {f.server.URL + "/mcp"}}
	var result struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		Error   string `json:"error"`
	}
	f.json(f.request(c, "POST", "/oauth/token", form.Encode(), "application/x-www-form-urlencoded", "", ""), want, &result)
	if want == 200 && (result.Access == "" || result.Refresh == "" || result.Refresh == grant.refresh) {
		f.t.Fatal("invalid refresh response")
	}
	if want == 400 && result.Error != "invalid_grant" {
		f.t.Fatal("missing invalid_grant")
	}
	return e2eOAuth{grant.client, result.Access, result.Refresh}
}
func TestE2E_OAuthGrantAndBlock(t *testing.T) {
	f := newE2E(t)
	browser := f.client()
	f.login(browser)
	me := f.me(browser)
	first := f.grant(browser, false)
	f.mcp(browser, first.access, 200)
	rotated := f.refresh(browser, first, 200)
	f.mcp(browser, rotated.access, 200)
	f.refresh(browser, first, 400)
	f.mcp(browser, rotated.access, 401)
	second := f.grant(browser, false)
	f.mcp(browser, second.access, 200)
	var grants []struct {
		ID   int64  `json:"id"`
		Name string `json:"client_name"`
	}
	f.json(f.request(browser, "GET", "/api/v1/me/grants", "", "", "", ""), 200, &grants)
	if len(grants) != 1 || grants[0].ID <= 0 || grants[0].Name == "" {
		t.Fatal("invalid grants shape")
	}
	f.status(f.request(browser, "DELETE", fmt.Sprintf("/api/v1/me/grants/%d", grants[0].ID), "", "", me.CSRF, ""), 204)
	f.mcp(browser, second.access, 401)
	third := f.grant(browser, false)
	personal := f.personal(browser, me.CSRF)
	f.mcp(browser, third.access, 200)
	f.mcp(browser, personal.Token, 200)
	f.me(browser)
	if err := runAdmin(t.Context(), []string{"account", "block", "--account", fmt.Sprint(me.ID)}, f.db, io.Discard); err != nil {
		t.Fatal("block account failed")
	}
	f.offset.Store(int64(11 * time.Second))
	f.status(f.request(browser, "GET", "/api/v1/me", "", "", "", ""), 401)
	f.mcp(browser, personal.Token, 401)
	f.mcp(browser, third.access, 401)
}

func TestE2E_OAuthLoginAndConsentRejections(t *testing.T) {
	f := newE2E(t)
	browser := f.client()
	grant := f.grant(browser, true)
	f.me(browser)
	f.mcp(browser, grant.access, 200)
}
