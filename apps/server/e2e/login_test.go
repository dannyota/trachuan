//go:build e2e

package e2e

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/oauth2"
)

const clientID = "e2e-client"
const clientSecret = "e2e-secret"

type loopbackJar struct{ http.CookieJar }

func secureURL(u *url.URL) *url.URL                     { copyURL := *u; copyURL.Scheme = "https"; return &copyURL }
func (j loopbackJar) Cookies(u *url.URL) []*http.Cookie { return j.CookieJar.Cookies(secureURL(u)) }
func (j loopbackJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.CookieJar.SetCookies(secureURL(u), cookies)
}
func client(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: loopbackJar{jar}, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func base(t *testing.T, key string) string {
	t.Helper()
	value := os.Getenv(key)
	if value == "" {
		t.Fatalf("%s is required", key)
	}
	return strings.TrimSuffix(value, "/")
}
func get(t *testing.T, c *http.Client, target string) *http.Response {
	t.Helper()
	r, err := c.Get(target)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	return r
}
func closeBody(r *http.Response) { _, _ = io.Copy(io.Discard, r.Body); _ = r.Body.Close() }
func fields(t *testing.T, r *http.Response) url.Values {
	t.Helper()
	defer closeBody(r)
	if r.StatusCode != 200 {
		t.Fatalf("form status %d", r.StatusCode)
	}
	doc, err := html.Parse(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	v := url.Values{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "input" {
			attrs := map[string]string{}
			for _, a := range n.Attr {
				attrs[a.Key] = a.Val
			}
			if attrs["type"] == "hidden" {
				v.Add(attrs["name"], attrs["value"])
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return v
}
func login(t *testing.T, c *http.Client, email string, verified bool) string {
	t.Helper()
	server := base(t, "E2E_SERVER_URL")
	google := base(t, "E2E_GOOGLE_URL")
	r := get(t, c, server+"/api/v1/auth/google/start?next=%2Faccount")
	loc := r.Header.Get("Location")
	closeBody(r)
	if r.StatusCode != 302 || !strings.HasPrefix(loc, google+"/") {
		t.Fatalf("start status %d", r.StatusCode)
	}
	v := fields(t, get(t, c, loc))
	v.Set("email", email)
	if verified {
		v.Set("email_verified", "on")
	}
	r, err := c.PostForm(google+"/authorize", v)
	if err != nil {
		t.Fatal(err)
	}
	loc = r.Header.Get("Location")
	closeBody(r)
	if r.StatusCode != 302 || !strings.HasPrefix(loc, server+"/") {
		t.Fatalf("authorize status %d", r.StatusCode)
	}
	r = get(t, c, loc)
	loc = r.Header.Get("Location")
	closeBody(r)
	if r.StatusCode != 302 {
		t.Fatalf("callback status %d", r.StatusCode)
	}
	if loc == "/account" {
		landing := get(t, c, server+loc)
		if landing.Request.URL.Path != "/account" {
			t.Fatal("incorrect landing path")
		}
		closeBody(landing)
	}
	return loc
}

type account struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}

func me(t *testing.T, c *http.Client) (int, account) {
	t.Helper()
	r := get(t, c, base(t, "E2E_SERVER_URL")+"/api/v1/me")
	defer closeBody(r)
	var result account
	if r.StatusCode == 200 {
		if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
	}
	return r.StatusCode, result
}
func TestLogin_Flow(t *testing.T) {
	first := client(t)
	if status, _ := me(t, first); status != 204 {
		t.Fatalf("signed out: %d", status)
	}
	if loc := login(t, first, "first@example.invalid", true); loc != "/account" {
		t.Fatalf("destination %q", loc)
	}
	status, a := me(t, first)
	if status != 200 || a.Email != "first@example.invalid" || a.ID <= 0 {
		t.Fatal("first account missing")
	}
	second := client(t)
	if loc := login(t, second, "second@example.invalid", true); loc != "/account" {
		t.Fatalf("destination %q", loc)
	}
	status, b := me(t, second)
	if status != 200 || b.Email != "second@example.invalid" || b.ID == a.ID {
		t.Fatal("second account mismatch")
	}
	unverified := client(t)
	if loc := login(t, unverified, "third@example.invalid", false); loc != "/login?error=login_failed" {
		t.Fatalf("destination %q", loc)
	}
	if status, _ := me(t, unverified); status != 204 {
		t.Fatalf("unverified account: %d", status)
	}
}
func TestToken_RejectsReusedCode(t *testing.T) {
	c := client(t)
	google := base(t, "E2E_GOOGLE_URL")
	redirect := base(t, "E2E_SERVER_URL") + "/api/v1/auth/google/callback"
	verifier := strings.Repeat("a", 43)
	q := url.Values{"client_id": {clientID}, "redirect_uri": {redirect}, "response_type": {"code"}, "scope": {"openid email"}, "code_challenge": {oauth2.S256ChallengeFromVerifier(verifier)}, "code_challenge_method": {"S256"}, "nonce": {"nonce"}, "state": {"state"}}
	v := fields(t, get(t, c, google+"/authorize?"+q.Encode()))
	v.Set("email", "reuse@example.invalid")
	v.Set("email_verified", "on")
	r, err := c.PostForm(google+"/authorize", v)
	if err != nil {
		t.Fatal(err)
	}
	location, err := r.Location()
	closeBody(r)
	if err != nil || r.StatusCode != 302 {
		t.Fatal("code not issued")
	}
	token := url.Values{"grant_type": {"authorization_code"}, "code": {location.Query().Get("code")}, "code_verifier": {verifier}, "redirect_uri": {redirect}, "client_id": {clientID}, "client_secret": {clientSecret}}
	for i, want := range []int{200, 400} {
		r, err = c.PostForm(google+"/token", token)
		if err != nil {
			t.Fatal(err)
		}
		closeBody(r)
		if r.StatusCode != want {
			t.Fatalf("exchange %d: got %d want %d", i, r.StatusCode, want)
		}
	}
}
