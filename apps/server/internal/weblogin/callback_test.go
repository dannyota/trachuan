package weblogin

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/weblogin/oidctest"
)

func TestCallback_RejectsBadStateNonceAudienceIssuerExpirySignature(t *testing.T) {
	for _, name := range []string{"state", "missing state", "nonce", "missing nonce", "audience", "issuer", "expiry", "signature", "missing subject", "missing email", "PKCE"} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			a := f.begin(t, "/")
			f.register(a, "test-code", func(c *oidctest.Claims) {
				switch name {
				case "nonce":
					c.Nonce = "wrong-nonce"
				case "missing nonce":
					c.Nonce = ""
				case "audience":
					c.Audience = "other-client"
				case "issuer":
					c.Issuer = "https://attacker.example"
				case "expiry":
					c.ExpiresAt = f.now.Add(-time.Hour)
				case "signature":
					c.BadSignature = true
				case "missing subject":
					c.Subject = ""
				case "missing email":
					c.Email = ""
				case "PKCE":
					c.Challenge = "wrong-challenge"
				}
			})
			if name == "state" {
				a.state = "wrong-state"
			}
			if name == "missing state" {
				a.state = ""
			}
			assertRejected(t, f, f.callback(a, "test-code"), "login_failed")
			if f.store.upserts != 0 {
				t.Error("account written before verification")
			}
			if (name == "state" || name == "missing state") && f.p.Count("/token") != 0 {
				t.Error("exchanged code before checking state")
			}
			assertRejected(t, f, f.callback(a, "test-code"), "login_expired")
		})
	}
}

func TestCallback_RejectsUnverifiedEmail(t *testing.T) {
	for _, verified := range []any{false, nil, "true", "false", 1} {
		t.Run("claim="+claimName(verified), func(t *testing.T) {
			f := newFixture(t)
			a := f.begin(t, "/")
			f.register(a, "test-code", func(c *oidctest.Claims) { c.EmailVerified = verified })
			assertRejected(t, f, f.callback(a, "test-code"), "login_failed")
			if f.store.upserts != 0 {
				t.Error("unverified email reached upsert")
			}
		})
	}
}
func claimName(v any) string {
	switch c := v.(type) {
	case bool:
		if c {
			return "true"
		}
		return "false"
	case string:
		return "string-" + c
	case int:
		return "number"
	default:
		return "missing"
	}
}

func TestCallback_ReplayAndConcurrentClaim(t *testing.T) {
	for _, name := range []string{"sequential", "concurrent"} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			a := f.begin(t, "/vn/search")
			f.register(a, "test-code", nil)
			const attempts = 12
			results := make([]*httptest.ResponseRecorder, attempts)
			if name == "sequential" {
				for i := range results {
					results[i] = f.callback(a, "test-code")
				}
			} else {
				start := make(chan struct{})
				var wg sync.WaitGroup
				for i := range results {
					wg.Go(func() { <-start; results[i] = f.callback(a, "test-code") })
				}
				close(start)
				wg.Wait()
			}
			var success, rejected int
			for _, w := range results {
				if w.Code == 302 && w.Header().Get("Location") == "/vn/search" {
					success++
				} else if w.Code == 302 && w.Header().Get("Location") == "/login?error=login_expired" {
					rejected++
				} else {
					t.Errorf("unexpected response=%d %q", w.Code, w.Header().Get("Location"))
				}
			}
			if success != 1 || rejected != attempts-1 || f.sessions.count() != 1 || f.store.upserts != 1 || f.p.Count("/token") != 1 {
				t.Errorf("success=%d rejected=%d sessions=%d upserts=%d exchanges=%d", success, rejected, f.sessions.count(), f.store.upserts, f.p.Count("/token"))
			}
		})
	}
}

func TestCallback_ExpiredTransaction(t *testing.T) {
	for _, after := range []time.Duration{10 * time.Minute, 11 * time.Minute} {
		t.Run(after.String(), func(t *testing.T) {
			f := newFixture(t)
			a := f.begin(t, "/")
			f.register(a, "test-code", nil)
			f.now = f.now.Add(after)
			assertRejected(t, f, f.callback(a, "test-code"), "login_expired")
			if f.p.Count("/token") != 0 || f.store.upserts != 0 {
				t.Error("expired transaction proceeded")
			}
		})
	}
}

func TestCallback_BlockedOrDeletedAccount(t *testing.T) {
	for _, a := range []GoogleAccount{{ID: 7, Blocked: true}, {ID: 7, DeleteRequested: true}} {
		t.Run(claimName(a.Blocked), func(t *testing.T) {
			f := newFixture(t)
			f.store.accounts["test-sub"] = a
			attempt := f.begin(t, "/")
			f.register(attempt, "test-code", nil)
			assertRejected(t, f, f.callback(attempt, "test-code"), "account_blocked")
		})
	}
}

func TestCallback_DatabaseFailureIs503AtEachStep(t *testing.T) {
	for _, name := range []string{"claim", "upsert", "presented-session revoke", "session create"} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			a := f.begin(t, "/")
			f.register(a, "test-code", nil)
			switch name {
			case "claim":
				f.store.claimErr = privateFailure()
			case "upsert":
				f.store.upsertErr = privateFailure()
			default:
				f.sessions.err = privateFailure()
			}
			w := f.callback(a, "test-code")
			assertUnavailable(t, f, w)
			if c := cookieNamed(t, w, "__Host-login"); c.MaxAge >= 0 {
				t.Error("login cookie not cleared")
			}
			f.store.claimErr = nil
			f.store.upsertErr = nil
			f.sessions.err = nil
			if name != "claim" {
				assertRejected(t, f, f.callback(a, "test-code"), "login_expired")
			}
		})
	}
}

func TestCallback_ProviderFailureIs503(t *testing.T) {
	for _, path := range []string{"/token", "/jwks"} {
		t.Run(path, func(t *testing.T) {
			f := newFixture(t)
			a := f.begin(t, "/")
			f.register(a, "test-code", nil)
			f.p.SetStatus(path, 503)
			w := f.callback(a, "test-code")
			assertUnavailable(t, f, w)
			if f.store.upserts != 0 {
				t.Error("provider failure reached account upsert")
			}
			f.p.SetStatus(path, 0)
			assertRejected(t, f, f.callback(a, "test-code"), "login_expired")
		})
	}
}

func TestCallback_BrowserStateCannotBeSpliced(t *testing.T) {
	f := newFixture(t)
	victim := f.begin(t, "/")
	attacker := f.begin(t, "/")
	f.register(attacker, "attacker-code", nil)
	victim.state = attacker.state
	assertRejected(t, f, f.callback(victim, "attacker-code"), "login_failed")
	if f.p.Count("/token") != 0 {
		t.Error("cross-browser state reached token endpoint")
	}
}

func TestCallback_UsesStoredRedirectURI(t *testing.T) {
	f := newFixture(t)
	a := f.begin(t, "/")
	f.register(a, "test-code", nil)
	f.h.oauth.RedirectURL = "https://other.example/callback"
	w := f.callback(a, "test-code")
	if w.Code != 302 || w.Header().Get("Location") != "/" || f.p.LastTokenForm().Get("redirect_uri") != a.tx.RedirectURI {
		t.Error("exchange did not retain stored redirect URI")
	}
}

func TestLogin_OnlyGETMutatesTransactions(t *testing.T) {
	f := newFixture(t)
	for _, handler := range []http.Handler{f.h.Start, f.h.Callback} {
		for _, method := range []string{http.MethodHead, http.MethodPost, http.MethodOptions} {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(method, startPath, nil))
			if w.Code != 405 || w.Header().Get("Allow") != "GET" {
				t.Errorf("method %s response=%d", method, w.Code)
			}
		}
	}
	if len(f.store.transactions) != 0 || f.store.claims != 0 {
		t.Error("non-GET changed transactions")
	}
}

func TestCallback_RevalidatesStoredNext(t *testing.T) {
	f := newFixture(t)
	a := f.begin(t, "/")
	f.register(a, "test-code", nil)
	tx := a.tx
	tx.NextPath = "//attacker.example"
	f.store.transactions[tx.HandleHash] = tx
	w := f.callbackQuery(a, url.Values{"state": {a.state}, "code": {"test-code"}})
	if w.Header().Get("Location") != "/" {
		t.Errorf("stored unsafe path used: %s", w.Header().Get("Location"))
	}
}
