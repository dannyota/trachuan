package weblogin

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"danny.vn/trachuan/apps/server/internal/weblogin/oidctest"
)

func TestLogin_NewAndReturningAccount(t *testing.T) {
	f := newFixture(t)
	for i := range 2 {
		a := f.begin(t, "/vn/search?topic=law")
		f.register(a, "test-code", nil)
		w := f.callback(a, "test-code")
		if w.Code != 302 || w.Header().Get("Location") != "/vn/search?topic=law" {
			t.Fatalf("login %d=%d %q", i, w.Code, w.Header().Get("Location"))
		}
		if c := cookieNamed(t, w, "__Host-login"); c.MaxAge >= 0 {
			t.Error("login cookie not cleared")
		}
		cookieNamed(t, w, "__Host-session")
	}
	if len(f.store.accounts) != 1 || f.store.upserts != 2 || f.sessions.count() != 2 {
		t.Fatalf("accounts=%d upserts=%d sessions=%d", len(f.store.accounts), f.store.upserts, f.sessions.count())
	}
	if f.sessions.accounts[0] != f.sessions.accounts[1] || f.sessions.accounts[0] != 1 {
		t.Error("returning subject changed account")
	}
	if len(f.sessions.presented) != 2 || f.sessions.presented[0] != "old-test-session" {
		t.Error("presented session not passed to creator")
	}
	if f.p.Count("/.well-known/openid-configuration") != 1 {
		t.Error("provider discovered more than once")
	}
}

func TestLogin_EmailFixedBySub(t *testing.T) {
	f := newFixture(t)
	for _, c := range []struct{ sub, email string }{{"first-sub", "first@example.test"}, {"first-sub", "new@example.test"}, {"second-sub", "new@example.test"}} {
		a := f.begin(t, "/")
		f.register(a, "test-code", func(v *oidctest.Claims) { v.Subject = c.sub; v.Email = c.email })
		if w := f.callback(a, "test-code"); w.Code != 302 || w.Header().Get("Location") != "/" {
			t.Fatalf("login=%d %s", w.Code, w.Body.String())
		}
	}
	if len(f.store.accounts) != 2 || f.store.emails["first-sub"] != "first@example.test" || f.store.accounts["first-sub"].ID == f.store.accounts["second-sub"].ID {
		t.Error("accounts not keyed by subject with fixed email")
	}
}

func TestStart_TransactionAndCookie(t *testing.T) {
	f := newFixture(t)
	a := f.begin(t, "/vn/search?q=1")
	c := a.cookie
	if c.Name != "__Host-login" || c.Path != "/" || c.Domain != "" || c.MaxAge != 600 || !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie=%+v", c)
	}
	values := []string{c.Value, a.state, a.nonce, a.tx.PKCEVerifier}
	seen := make(map[string]bool)
	for _, v := range values {
		b, err := base64.RawURLEncoding.DecodeString(v)
		if err != nil || len(b) != 32 || seen[v] {
			t.Errorf("not an independent 32-byte base64url token")
		}
		seen[v] = true
	}
	if len(a.tx.HandleHash) != 64 || a.tx.HandleHash == c.Value || a.tx.State != a.state || a.tx.Nonce != a.nonce || a.tx.NextPath != "/vn/search?q=1" || a.tx.RedirectURI != publicURL+callbackPath || !a.tx.CreatedAt.Equal(f.now) || !a.tx.ExpiresAt.Equal(f.now.Add(10*time.Minute)) {
		t.Errorf("stored transaction does not match start")
	}
	if a.challenge != oauth2.S256ChallengeFromVerifier(a.tx.PKCEVerifier) {
		t.Error("incorrect PKCE challenge")
	}
	w := httptest.NewRecorder()
	f.h.Start.ServeHTTP(w, httptest.NewRequest(http.MethodGet, startPath, nil))
	u, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if u.Scheme+"://"+u.Host != f.p.URL || q.Get("scope") != "openid email" || q.Get("response_type") != "code" || q.Get("client_id") != oidctest.ClientID || q.Get("code_challenge_method") != "S256" || q.Get("redirect_uri") != publicURL+callbackPath {
		t.Errorf("authorization request=%v", q)
	}
	f.register(a, "test-code", nil)
	if w = f.callback(a, "test-code"); w.Header().Get("Location") != a.tx.NextPath {
		t.Fatal("valid login failed")
	}
	form := f.p.LastTokenForm()
	if form.Get("code_verifier") != a.tx.PKCEVerifier || form.Get("redirect_uri") != a.tx.RedirectURI {
		t.Error("exchange did not use stored PKCE and redirect URI")
	}
}

func TestLogin_ErrorsAreFixedCodes(t *testing.T) {
	for _, name := range []string{"provider error", "invalid code", "missing code", "missing token", "missing cookie", "malformed cookie", "unknown cookie", "duplicate cookie", "duplicate state", "duplicate code", "malformed query"} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			a := f.begin(t, "//attacker.example")
			f.register(a, "test-code", func(c *oidctest.Claims) { c.OmitIDToken = name == "missing token" })
			q := url.Values{"state": {a.state}, "code": {"test-code"}}
			want := "login_failed"
			switch name {
			case "provider error":
				q.Set("error", "provider-private-detail")
				q.Set("error_description", "private-description")
			case "invalid code":
				q.Set("code", "unknown-code")
			case "missing code":
				q.Del("code")
			case "missing cookie":
				a.cookie = nil
				want = "login_expired"
			case "malformed cookie":
				a.cookie = &http.Cookie{Name: "__Host-login", Value: "bad"}
				want = "login_expired"
			case "unknown cookie":
				a.cookie = &http.Cookie{Name: "__Host-login", Value: strings.Repeat("a", 43)}
				want = "login_expired"
			case "duplicate state":
				q.Add("state", a.state)
			case "duplicate code":
				q.Add("code", "test-code")
			}
			var w *httptest.ResponseRecorder
			if name == "duplicate cookie" || name == "malformed query" {
				path := callbackPath + "?" + q.Encode()
				if name == "malformed query" {
					path += "&bad=%zz"
				}
				r := httptest.NewRequest(http.MethodGet, path, nil)
				r.AddCookie(a.cookie)
				if name == "duplicate cookie" {
					r.AddCookie(a.cookie)
					want = "login_expired"
				}
				w = httptest.NewRecorder()
				f.h.Callback.ServeHTTP(w, r)
			} else {
				w = f.callbackQuery(a, q)
			}
			assertRejected(t, f, w, want)
		})
	}
}

func TestLogin_NoSecretsInLogs(t *testing.T) {
	var logs bytes.Buffer
	oldSlog := slog.Default()
	oldLogWriter := log.Writer()
	oldFlags := log.Flags()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	log.SetOutput(&logs)
	log.SetFlags(0)
	t.Cleanup(func() { slog.SetDefault(oldSlog); log.SetOutput(oldLogWriter); log.SetFlags(oldFlags) })
	f := newFixture(t)
	a := f.begin(t, "/")
	f.register(a, "private-auth-code", nil)
	f.store.upsertErr = privateFailure()
	w := f.callback(a, "private-auth-code")
	assertUnavailable(t, f, w)
	forbidden := []string{a.cookie.Value, a.state, a.nonce, a.tx.PKCEVerifier, "test-client-secret", "private-auth-code", "first@example.test", "private-database-detail", "old-test-session"}
	for _, v := range forbidden {
		if strings.Contains(logs.String(), v) || strings.Contains(w.Body.String()+w.Header().Get("Location"), v) {
			t.Errorf("sensitive value leaked")
		}
	}
}

func TestNew_RequiresDependenciesAndDiscoversAtStartup(t *testing.T) {
	p := oidctest.New(t)
	valid := Config{ClientID: oidctest.ClientID, ClientSecret: "test-client-secret", PublicURL: publicURL, Issuer: p.URL, Store: newFakeStore(), Sessions: &fakeSessions{}}
	for _, name := range []string{"id", "secret", "url", "store", "sessions", "relative URL", "URL query", "URL fragment", "URL userinfo"} {
		t.Run(name, func(t *testing.T) {
			c := valid
			switch name {
			case "id":
				c.ClientID = ""
			case "secret":
				c.ClientSecret = ""
			case "url":
				c.PublicURL = ""
			case "store":
				c.Store = nil
			case "sessions":
				c.Sessions = nil
			case "relative URL":
				c.PublicURL = "/relative"
			case "URL query":
				c.PublicURL += "?x=1"
			case "URL fragment":
				c.PublicURL += "#x"
			case "URL userinfo":
				c.PublicURL = "https://user@law.example"
			}
			if _, err := New(t.Context(), c); err == nil {
				t.Error("invalid config accepted")
			}
		})
	}
	if _, err := New(t.Context(), valid); err != nil {
		t.Fatal(err)
	}
	if p.Count("/.well-known/openid-configuration") != 1 {
		t.Error("discovery not performed once at startup")
	}
	p.SetStatus("/.well-known/openid-configuration", 503)
	if _, err := New(t.Context(), valid); err == nil {
		t.Error("discovery failure accepted")
	}
}

func TestStart_FailsClosed(t *testing.T) {
	for _, name := range []string{"database", "random handle", "random state", "random nonce"} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			c := Config{ClientID: oidctest.ClientID, ClientSecret: "test-client-secret", PublicURL: publicURL, Issuer: f.p.URL, Store: f.store, Sessions: f.sessions}
			switch name {
			case "database":
				f.store.createErr = privateFailure()
			case "random handle":
				c.Rand = io.LimitReader(strings.NewReader(""), 0)
			case "random state":
				c.Rand = bytes.NewReader(make([]byte, 32))
			case "random nonce":
				c.Rand = bytes.NewReader(make([]byte, 64))
			}
			h, err := New(context.Background(), c)
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			h.Start.ServeHTTP(w, httptest.NewRequest(http.MethodGet, startPath, nil))
			assertUnavailable(t, f, w)
			if len(f.store.transactions) != 0 {
				t.Error("transaction stored after failure")
			}
			for _, v := range w.Result().Cookies() {
				if v.Name == "__Host-login" && v.MaxAge > 0 {
					t.Error("live cookie on failed start")
				}
			}
		})
	}
}
