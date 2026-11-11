package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func ioBody(s string) io.ReadCloser { return io.NopCloser(strings.NewReader(s)) }

func TestConsent_RequiresSessionAndRoundTripsLogin(t *testing.T) {
	d := newConsentDB()
	path, cookie := pendingConsent(t, d)
	w := consentResponse(d, consentRequest(path, "GET", cookie, false, nil))
	u, _ := url.Parse(w.Header().Get("Location"))
	if w.Code != 303 || u.Path != "/api/v1/auth/google/start" || u.Query().Get("next") != path || len(w.Result().Cookies()) != 0 {
		t.Fatalf("login %d %s", w.Code, u)
	}
	w = consentResponse(d, consentRequest(path, "GET", cookie, true, nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "<form") {
		t.Fatalf("consent %d", w.Code)
	}
}
func TestConsent_BrowserBindingAndForgery(t *testing.T) {
	for _, tc := range []string{"no cookie", "other cookie", "duplicate cookie", "no token", "wrong token", "no origin", "wrong origin", "duplicate origin", "wrong type", "repeat token", "query token", "bad decision", "expired", "claimed", "other handle"} {
		t.Run(tc, func(t *testing.T) {
			d := newConsentDB()
			path, cookie := pendingConsent(t, d)
			f := formFor(path, "approve")
			r := consentRequest(path, "POST", cookie, true, f)
			switch tc {
			case "no cookie":
				r.Header.Del("Cookie")
				r.AddCookie(&http.Cookie{Name: "__Host-session", Value: strings.Repeat("A", 43)})
			case "other cookie":
				r.Header.Del("Cookie")
				r.AddCookie(&http.Cookie{Name: "__Host-oauth", Value: strings.Repeat("B", 43)})
				r.AddCookie(&http.Cookie{Name: "__Host-session", Value: strings.Repeat("A", 43)})
			case "duplicate cookie":
				r.AddCookie(cookie)
			case "no token":
				f.Del("csrf_token")
			case "wrong token":
				f.Set("csrf_token", "bad")
			case "no origin":
				r.Header.Del("Origin")
			case "wrong origin":
				r.Header.Set("Origin", "https://attacker.example")
			case "duplicate origin":
				r.Header.Add("Origin", consentBase)
			case "wrong type":
				r.Header.Set("Content-Type", "text/plain")
			case "repeat token":
				f.Add("csrf_token", f.Get("csrf_token"))
			case "query token":
				r.URL.RawQuery = f.Encode()
				f.Del("csrf_token")
			case "bad decision":
				f.Set("decision", "perhaps")
			case "expired":
				d.pending.ExpiresAt = d.pending.CreatedAt
			case "claimed":
				d.claimed = true
			case "other handle":
				r.URL.Path = "/oauth/consent/" + strings.Repeat("C", 43)
			}
			r.Body = http.NoBody
			if len(f) > 0 {
				r.Body = ioBody(f.Encode())
			}
			w := consentResponse(d, r)
			if w.Code < 400 || d.code.CodeHash != "" || (d.claimed && tc != "claimed") {
				t.Fatalf("forgery status %d", w.Code)
			}
		})
	}
}
func TestConsent_ShowsHostsAndWarnings(t *testing.T) {
	for _, lang := range []string{"vi", "en"} {
		for _, kind := range []string{"dcr", "cimd"} {
			t.Run(lang+kind, func(t *testing.T) {
				d := newConsentDB()
				path, cookie := pendingConsent(t, d)
				d.client.Kind = kind
				if kind == "cimd" {
					d.client.ClientID = "https://metadata.example/client"
				}
				w := consentResponse(d, consentRequest(path+"?lang="+lang, "GET", cookie, true, nil))
				body := w.Body.String()
				for _, want := range []string{"localhost", "&lt;script&gt;", "mcp:read"} {
					if !strings.Contains(body, want) {
						t.Errorf("missing %s", want)
					}
				}
				if strings.Contains(body, "<script>") || strings.Contains(body, "session-secret-fixture") {
					t.Fatal("unsafe output")
				}
				if kind == "cimd" && !strings.Contains(body, "metadata.example") {
					t.Fatal("missing CIMD host")
				}
				if lang == "en" {
					if !strings.Contains(body, "application on this computer") || (kind == "dcr" && !strings.Contains(body, "name is unverified")) {
						t.Fatal("missing English warnings")
					}
				} else if !strings.Contains(body, "máy tính này") || (kind == "dcr" && !strings.Contains(body, "chưa được xác minh")) {
					t.Fatal("missing Vietnamese warnings")
				}
			})
		}
	}
}
func TestConsent_HeadersAndCSP(t *testing.T) {
	d := newConsentDB()
	path, cookie := pendingConsent(t, d)
	w := consentResponse(d, consentRequest(path, "GET", cookie, true, nil))
	h := w.Header()
	if h.Get("Cache-Control") != "no-store" || h.Get("X-Frame-Options") != "DENY" || h.Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("security headers")
	}
	for _, part := range []string{"default-src 'none'", "style-src 'self'", "frame-ancestors 'none'", "form-action 'self' http://localhost"} {
		if !strings.Contains(h.Get("Content-Security-Policy"), part) {
			t.Errorf("missing CSP %s", part)
		}
	}
	if strings.Contains(w.Body.String(), "<script") {
		t.Fatal("inline scripts")
	}
	body := w.Body.String()
	start := strings.Index(body, "<style>")
	end := strings.Index(body, "</style>")
	if start < 0 || end < start {
		t.Fatal("missing embedded stylesheet")
	}
	digest := sha256.Sum256([]byte(body[start+7 : end]))
	want := "'sha256-" + base64.StdEncoding.EncodeToString(digest[:]) + "'"
	if !strings.Contains(h.Get("Content-Security-Policy"), want) {
		t.Fatal("stylesheet hash not allowed by CSP")
	}

}
func TestConsent_ApproveDeny303WithStateIss(t *testing.T) {
	for _, decision := range []string{"approve", "deny"} {
		t.Run(decision, func(t *testing.T) {
			d := newConsentDB()
			path, cookie := pendingConsent(t, d)
			w := consentResponse(d, consentRequest(path, "POST", cookie, true, formFor(path, decision)))
			u, _ := url.Parse(w.Header().Get("Location"))
			v := u.Query()
			if w.Code != 303 || u.Host != "localhost" || v.Get("state") != "opaque & state" || v.Get("iss") != consentBase || !d.claimed || d.pending.AccountID == nil || *d.pending.AccountID != 7 {
				t.Fatalf("decision %d", w.Code)
			}
			cookies := w.Result().Cookies()
			if len(cookies) != 1 || cookies[0].Name != "__Host-oauth" || cookies[0].MaxAge != -1 {
				t.Fatal("cookie not cleared")
			}
			if decision == "approve" {
				if len(v.Get("code")) != 43 || d.code.CodeHash != consentHash(v.Get("code")) || d.code.AuthEpoch != 3 || d.code.GrantID != 9 || d.code.ClientID != "fixture" || d.code.AccountID != 7 || d.code.RedirectUri != d.pending.RedirectUri || d.code.CodeChallenge != d.pending.CodeChallenge || d.code.Resource != d.pending.Resource || d.code.Scope != "mcp:read" {
					t.Fatal("code bindings")
				}
			} else if v.Get("error") != "access_denied" || d.code.CodeHash != "" || d.grants != 0 {
				t.Fatal("denial issued authority")
			}
		})
	}
}
func TestConsent_GrantCap(t *testing.T) {
	d := newConsentDB()
	path, cookie := pendingConsent(t, d)
	d.grants = 20
	w := consentResponse(d, consentRequest(path+"?lang=en", "POST", cookie, true, formFor(path, "approve")))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "revoke") || d.claimed || d.code.CodeHash != "" || d.grants != 20 {
		t.Fatalf("cap %d", w.Code)
	}
}
func TestConsent_ClaimOnceUnderRace(t *testing.T) {
	d := newConsentDB()
	path, cookie := pendingConsent(t, d)
	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for range 2 {
		wg.Go(func() {
			statuses <- consentResponse(d, consentRequest(path, "POST", cookie, true, formFor(path, "approve"))).Code
		})
	}
	wg.Wait()
	close(statuses)
	success := 0
	for code := range statuses {
		if code == 303 {
			success++
		} else if code != 400 {
			t.Fatalf("status %d", code)
		}
	}
	if success != 1 || d.grants != 1 {
		t.Fatalf("success count %d grants %d", success, d.grants)
	}
}
func TestConsent_DatabaseFailuresAndUnknownCommit(t *testing.T) {
	for _, commit := range []bool{false, true} {
		d := newConsentDB()
		path, cookie := pendingConsent(t, d)
		d.fail = !commit
		d.commitFail = commit
		w := consentResponse(d, consentRequest(path, "POST", cookie, true, formFor(path, "approve")))
		if w.Code != 503 || w.Header().Get("Location") != "" || len(w.Result().Cookies()) != 0 || !strings.Contains(w.Body.String(), "temporarily_unavailable") {
			t.Fatalf("failure %d", w.Code)
		}
	}
}

func TestConsent_GrantReuseAtCap(t *testing.T) {
	d := newConsentDB()
	path, cookie := pendingConsent(t, d)
	d.grants = 20
	d.existingGrant = true
	w := consentResponse(d, consentRequest(path, "POST", cookie, true, formFor(path, "approve")))
	if w.Code != 303 || d.grants != 20 {
		t.Fatalf("reuse status %d grants %d", w.Code, d.grants)
	}
}
func TestConsent_QueryFailuresRollBack(t *testing.T) {
	for _, query := range []string{"GetClient", "LockCredentialAccount", "ClaimRequest", "UpsertGrant", "CountLiveGrants", "CreateCode", "FROM web_session"} {
		t.Run(query, func(t *testing.T) {
			d := newConsentDB()
			path, cookie := pendingConsent(t, d)
			d.failQuery = query
			w := consentResponse(d, consentRequest(path, "POST", cookie, true, formFor(path, "approve")))
			if w.Code != 503 || d.claimed || d.grants != 0 || d.code.CodeHash != "" || w.Header().Get("Location") != "" {
				t.Fatalf("query failure status %d", w.Code)
			}
		})
	}
}
func TestConsent_BlockedOrDeletedAccountCannotApprove(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		d := newConsentDB()
		path, cookie := pendingConsent(t, d)
		d.blocked = blocked
		d.deleted = !blocked
		w := consentResponse(d, consentRequest(path, "POST", cookie, true, formFor(path, "approve")))
		if w.Code != 400 || d.claimed || d.grants != 0 || d.code.CodeHash != "" {
			t.Fatalf("ineligible account status %d", w.Code)
		}
	}
}

func TestConsent_StaleCachedSessionCannotMintCurrentEpochCode(t *testing.T) {
	d := newConsentDB()
	path, cookie := pendingConsent(t, d)
	d.staleSession = true
	w := consentResponse(d, consentRequest(path, "POST", cookie, true, formFor(path, "approve")))
	if w.Code != 400 || d.code.CodeHash != "" || d.claimed {
		t.Fatalf("stale cached session status %d", w.Code)
	}
}
func TestConsent_FormTokenUsesScopedHMAC(t *testing.T) {
	if got := consentFormToken("session-secret-fixture", strings.Repeat("A", 43)); got != "TjtYok-lzkyyv_N8qJ_pftE7D4Y1OFdcSElfUDVN33c" {
		t.Fatal("wrong form HMAC")
	}
}
