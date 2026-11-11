package weblogin

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/weblogin/oidctest"
	"github.com/coreos/go-oidc/v3/oidc"
)

type discoveryTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (d discoveryTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host == "accounts.google.com" {
		r = r.Clone(r.Context())
		r.URL.Scheme = d.target.Scheme
		r.URL.Host = d.target.Host
	}
	return d.base.RoundTrip(r)
}

func TestCallback_GoogleIssuerForms(t *testing.T) {
	for _, tc := range []struct {
		name, configured, claim string
		accept                  bool
	}{
		{"google canonical", googleIssuer, googleIssuer, true},
		{"google legacy", googleIssuer, "accounts.google.com", true},
		{"google third value", googleIssuer, "https://accounts.google.com.attacker.example", false},
		{"override exact", "override", "override", true},
		{"override rejects legacy", "override", "accounts.google.com", false},
		{"override rejects google", "override", googleIssuer, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			configured, claim := tc.configured, tc.claim
			if configured == "override" {
				configured = f.p.URL
			}
			if claim == "override" {
				claim = f.p.URL
			}
			f.p.SetIssuer(configured)
			target, err := url.Parse(f.p.URL)
			if err != nil {
				t.Fatal(err)
			}
			client := &http.Client{Transport: discoveryTransport{target: target, base: http.DefaultTransport}}
			f.h, err = New(oidc.ClientContext(t.Context(), client), Config{ClientID: oidctest.ClientID, ClientSecret: "test-client-secret", PublicURL: publicURL, Issuer: configured, Store: f.store, Sessions: f.sessions, Clock: func() time.Time { return f.now }})
			if err != nil {
				t.Fatal(err)
			}
			a := f.begin(t, "/")
			f.register(a, "issuer-code", func(c *oidctest.Claims) { c.Issuer = claim })
			w := f.callback(a, "issuer-code")
			if tc.accept {
				if w.Code != 302 || w.Header().Get("Location") != "/" || f.sessions.count() != 1 {
					t.Errorf("valid issuer rejected: %d %q", w.Code, w.Header().Get("Location"))
				}
			} else {
				assertRejected(t, f, w, "login_failed")
			}
		})
	}
}

func TestCallback_ReusesProviderKeys(t *testing.T) {
	for _, concurrent := range []bool{false, true} {
		t.Run(claimName(concurrent), func(t *testing.T) {
			f := newFixture(t)
			const n = 12
			attempts := make([]attempt, n)
			for i := range attempts {
				attempts[i] = f.begin(t, "/")
				f.register(attempts[i], fmt.Sprintf("code-%d", i), nil)
			}
			results := make([]*httptest.ResponseRecorder, n)
			if concurrent {
				start := make(chan struct{})
				var wg sync.WaitGroup
				for i := range attempts {
					wg.Go(func() { <-start; results[i] = f.callback(attempts[i], fmt.Sprintf("code-%d", i)) })
				}
				close(start)
				wg.Wait()
			} else {
				for i := range attempts {
					results[i] = f.callback(attempts[i], fmt.Sprintf("code-%d", i))
				}
			}
			for _, w := range results {
				if w.Code != 302 || w.Header().Get("Location") != "/" {
					t.Errorf("callback=%d %q", w.Code, w.Header().Get("Location"))
				}
			}
			if got := f.p.Count("/jwks"); got != 1 {
				t.Errorf("JWKS requests=%d, want 1", got)
			}
			if f.sessions.count() != n {
				t.Errorf("sessions=%d", f.sessions.count())
			}
		})
	}
}

func TestCallback_ProviderAuthorizationErrors(t *testing.T) {
	for _, tc := range []struct {
		code        string
		unavailable bool
	}{
		{"server_error", true}, {"temporarily_unavailable", true}, {"access_denied", false}, {"other", false},
	} {
		t.Run(tc.code, func(t *testing.T) {
			f := newFixture(t)
			a := f.begin(t, "/")
			w := f.callbackQuery(a, url.Values{"state": {a.state}, "error": {tc.code}, "error_description": {"provider-private-detail"}})
			if tc.unavailable {
				assertUnavailable(t, f, w)
			} else {
				assertRejected(t, f, w, "login_failed")
			}
			if f.p.Count("/token") != 0 {
				t.Error("authorization error reached token endpoint")
			}
		})
	}
	for _, code := range []string{"server_error", "temporarily_unavailable"} {
		t.Run(code+" bad state", func(t *testing.T) {
			f := newFixture(t)
			a := f.begin(t, "/")
			w := f.callbackQuery(a, url.Values{"state": {"wrong"}, "error": {code}})
			assertRejected(t, f, w, "login_failed")
		})
	}
}

func TestCallback_MalformedJWKSUnavailable(t *testing.T) {
	f := newFixture(t)
	a := f.begin(t, "/")
	f.register(a, "test-code", nil)
	f.p.SetBrokenJWKS(true)
	assertUnavailable(t, f, f.callback(a, "test-code"))
	if f.store.upserts != 0 {
		t.Error("malformed JWKS reached account upsert")
	}
}

func TestCallback_CachedKeysIsolateProviderFailure(t *testing.T) {
	f := newFixture(t)
	warm := f.begin(t, "/")
	f.register(warm, "warm", nil)
	if w := f.callback(warm, "warm"); w.Code != 302 || w.Header().Get("Location") != "/" {
		t.Fatal("warm login failed")
	}
	good := f.begin(t, "/")
	f.register(good, "good", nil)
	bad := f.begin(t, "/")
	f.register(bad, "bad", func(c *oidctest.Claims) { c.BadSignature = true })
	f.p.SetStatus("/jwks", 503)
	var responses [2]*httptest.ResponseRecorder
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() { <-start; responses[0] = f.callback(good, "good") })
	wg.Go(func() { <-start; responses[1] = f.callback(bad, "bad") })
	close(start)
	wg.Wait()
	if responses[0].Code != 302 || responses[0].Header().Get("Location") != "/" {
		t.Errorf("cached valid token=%d %q", responses[0].Code, responses[0].Header().Get("Location"))
	}
	if responses[1].Code != 503 || responses[1].Header().Get("Location") != "" {
		t.Errorf("failed key refresh=%d %q", responses[1].Code, responses[1].Header().Get("Location"))
	}
	for _, c := range responses[1].Result().Cookies() {
		if c.Name == "__Host-session" {
			t.Error("session cookie on key failure")
		}
	}
}
