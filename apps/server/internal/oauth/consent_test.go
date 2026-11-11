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

	"golang.org/x/net/html"
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
	for _, tc := range []string{"no cookie", "other cookie", "duplicate cookie", "no token", "wrong token", "no origin", "null origin", "wrong origin", "duplicate origin", "wrong type", "repeat token", "query token", "bad decision", "expired", "claimed", "other handle"} {
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
			case "null origin":
				r.Header.Set("Origin", "null")
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
	if h.Get("Cache-Control") != "no-store" || h.Get("X-Frame-Options") != "DENY" || h.Get("Referrer-Policy") != "same-origin" {
		t.Fatal("security headers")
	}
	for _, part := range []string{"default-src 'none'", "style-src 'self'", "frame-ancestors 'none'"} {
		if !strings.Contains(h.Get("Content-Security-Policy"), part) {
			t.Errorf("missing CSP %s", part)
		}
	}
	if strings.Contains(h.Get("Content-Security-Policy"), "form-action") {
		t.Fatal("CSP must allow the decision redirect")
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

func TestConsent_BrowserDecisionAndLoopbackReturns(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "[::1]", "localhost"} {
		for _, decision := range []string{"approve", "deny", "error"} {
			t.Run(host+"/"+decision, func(t *testing.T) {
				d := newConsentDB()
				redirect := "http://" + host + ":43123/callback?existing=value"
				d.client.RedirectUris = []string{redirect}
				v := authorizeValues()
				v.Set("redirect_uri", redirect)
				if decision == "error" {
					v.Set("scope", "invalid")
					w := authorizeFor(t, d, v)
					if w.Code != 400 || w.Header().Get("Location") != "" || strings.Contains(w.Header().Get("Content-Security-Policy"), "form-action") {
						t.Fatalf("error page status %d", w.Code)
					}
					u := errorReturnLink(t, w.Body.String())
					if u.Host != host+":43123" || u.Query().Get("existing") != "value" || u.Query().Get("error") != "invalid_scope" || u.Query().Get("state") != "opaque & state" || u.Query().Get("iss") != consentBase {
						t.Fatal("invalid error link")
					}
					return
				}
				start := authorizeFor(t, d, v)
				if start.Code != 303 {
					t.Fatalf("authorize %d", start.Code)
				}
				path := start.Header().Get("Location")
				cookie := start.Result().Cookies()[0]
				page := consentResponse(d, consentRequest(path, "GET", cookie, true, nil))
				if page.Code != 200 {
					t.Fatalf("page %d", page.Code)
				}
				if strings.Contains(page.Header().Get("Content-Security-Policy"), "form-action") {
					t.Fatal("CSP blocks loopback decisions")
				}
				post := consentRequest(path, "POST", cookie, true, url.Values{"decision": {decision}, "csrf_token": {pageFormToken(t, page.Body.String())}})
				switch page.Header().Get("Referrer-Policy") {
				case "same-origin":
					post.Header.Set("Origin", consentBase)
					post.Header.Set("Referer", consentBase+path)
				case "no-referrer":
					post.Header.Set("Origin", "null")
					post.Header.Del("Referer")
				default:
					t.Fatal("unexpected referrer policy")
				}
				response := consentResponse(d, post)
				if response.Code != 303 || strings.Contains(response.Header().Get("Content-Security-Policy"), "form-action") {
					t.Fatalf("browser decision status %d", response.Code)
				}
				u, err := url.Parse(response.Header().Get("Location"))
				if err != nil {
					t.Fatal(err)
				}
				if u.Host != host+":43123" || u.Query().Get("existing") != "value" || u.Query().Get("state") != "opaque & state" || u.Query().Get("iss") != consentBase {
					t.Fatal("invalid decision return")
				}
				if decision == "approve" && u.Query().Get("code") == "" {
					t.Fatal("missing code")
				}
				if decision == "deny" && u.Query().Get("error") != "access_denied" {
					t.Fatal("missing denial")
				}
			})
		}
	}
}

func pageFormToken(t *testing.T, body string) string {
	t.Helper()
	root, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	token := ""
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "input" {
			attrs := map[string]string{}
			for _, a := range n.Attr {
				attrs[a.Key] = a.Val
			}
			if attrs["name"] == "csrf_token" {
				token = attrs["value"]
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	if token == "" {
		t.Fatal("missing consent token")
	}
	return token
}

func TestConsent_RejectsChangedClientGrantOnApproval(t *testing.T) {
	d := newConsentDB()
	d.client.ClientID = "https://client.example/metadata"
	d.client.Kind = "cimd"
	v := authorizeValues()
	v.Set("client_id", d.client.ClientID)
	start := authorizeFor(t, d, v)
	if start.Code != 303 {
		t.Fatalf("authorize status %d", start.Code)
	}
	path, cookie := start.Header().Get("Location"), start.Result().Cookies()[0]
	d.client.GrantTypes = []string{"refresh_token"}
	w := consentResponse(d, consentRequest(path, "POST", cookie, true, formFor(path, "approve")))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "unauthorized_client") || w.Header().Get("Location") != "" || d.claimed || d.grants != 0 || d.code.CodeHash != "" {
		t.Fatalf("refresh-only approval: %d", w.Code)
	}
}

func TestConsent_BrowserOriginTracksReferrerPolicy(t *testing.T) {
	d := newConsentDB()
	path, cookie := pendingConsent(t, d)
	page := consentResponse(d, consentRequest(path, "GET", cookie, true, nil))
	request := consentRequest(path, "POST", cookie, true, url.Values{"decision": {"approve"}, "csrf_token": {pageFormToken(t, page.Body.String())}})
	if page.Header().Get("Referrer-Policy") == "no-referrer" {
		request.Header.Set("Origin", "null")
		request.Header.Del("Referer")
	}
	w := consentResponse(d, request)
	if w.Code != 303 {
		t.Fatalf("browser form with %q policy returned %d", page.Header().Get("Referrer-Policy"), w.Code)
	}
}

func TestConsent_ProductDisplayName(t *testing.T) {
	for _, tc := range []struct{ lang, title, prompt, failure string }{
		{"vi", "Cho phép truy cập · Tra Chuẩn", "Cho phép ứng dụng truy cập tài khoản Tra Chuẩn của bạn?", "Không thể cấp quyền truy cập · Tra Chuẩn"},
		{"en", "Authorize access · Tra Chuẩn", "Allow this app to access your Tra Chuẩn account?", "Cannot authorize access · Tra Chuẩn"},
	} {
		t.Run(tc.lang, func(t *testing.T) {
			d := newConsentDB()
			path, cookie := pendingConsent(t, d)
			w := consentResponse(d, consentRequest(path+"?lang="+tc.lang, "GET", cookie, true, nil))
			for _, want := range []string{"<title>" + tc.title + "</title>", tc.prompt} {
				if !strings.Contains(w.Body.String(), want) {
					t.Errorf("missing %q", want)
				}
			}
			w = consentResponse(d, consentRequest(path+"?lang="+tc.lang, "POST", cookie, true, nil))
			if !strings.Contains(w.Body.String(), "<title>"+tc.failure+"</title>") {
				t.Errorf("missing failure title %q", tc.failure)
			}
		})
	}
}
