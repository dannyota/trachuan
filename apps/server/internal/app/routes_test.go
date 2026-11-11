package app

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/accountapi"
	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/mcpapi"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/weblogin"
	"danny.vn/trachuan/apps/server/internal/websession"
	"golang.org/x/oauth2"
)

type routeSessionStore struct{ websession.Store }

func (routeSessionStore) GetSession(_ context.Context, hash string) (store.GetSessionRow, error) {
	now := time.Now()
	return store.GetSessionRow{ID: 1, AccountID: 1, TokenHash: hash, CsrfSecret: strings.Repeat("A", 43), CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(time.Hour)}, nil
}

type routeLoginStore struct{ claims int }

func (*routeLoginStore) CreateLoginTransaction(context.Context, weblogin.LoginTransaction) error {
	return nil
}
func (s *routeLoginStore) ClaimLoginTransaction(context.Context, weblogin.ClaimLoginTransactionParams) (weblogin.LoginTransaction, error) {
	s.claims++
	return weblogin.LoginTransaction{}, sql.ErrNoRows
}
func (*routeLoginStore) UpsertGoogleAccount(context.Context, weblogin.UpsertGoogleAccountParams) (weblogin.GoogleAccount, error) {
	panic("unexpected account lookup")
}

type discoveryRoundTrip struct{}

func (discoveryRoundTrip) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"issuer":"https://issuer.test","authorization_endpoint":"https://issuer.test/auth","token_endpoint":"https://issuer.test/token","jwks_uri":"https://issuer.test/keys"}`))}, nil
}

type routeLimiter struct{ calls []limits.Class }

func (l *routeLimiter) Allow(_ int64, c limits.Class) (bool, time.Duration) {
	l.calls = append(l.calls, c)
	return false, time.Second
}

type routeAccountStore struct{ accountapi.Store }

type routeCase struct {
	method, path string
	class        limits.Class
}

func protectedRoutes() []routeCase {
	return []routeCase{
		{"POST", "/api/v1/auth/logout", limits.Read}, {"GET", "/api/v1/me", limits.Read}, {"DELETE", "/api/v1/me", limits.Read},
		{"GET", "/api/v1/me/grants", limits.Read}, {"DELETE", "/api/v1/me/grants/1", limits.Read}, {"GET", "/api/v1/me/sessions", limits.Read}, {"DELETE", "/api/v1/me/sessions/1", limits.Read}, {"GET", "/api/v1/me/tokens", limits.Read}, {"POST", "/api/v1/me/tokens", limits.Read}, {"DELETE", "/api/v1/me/tokens/1", limits.Read},
		{"POST", "/api/v1/search", limits.Search}, {"POST", "/api/v1/read", limits.Read}, {"GET", "/api/v1/documents/key", limits.Read}, {"GET", "/api/v1/status", limits.Read},
	}
}
func routeFixture(t *testing.T) (http.Handler, *routeLimiter, *routeLoginStore) {
	t.Helper()
	sessions := &websession.Manager{Store: routeSessionStore{}, PublicURL: "http://localhost"}
	ls := &routeLoginStore{}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{Transport: discoveryRoundTrip{}})
	login, err := weblogin.New(ctx, weblogin.Config{ClientID: "id", ClientSecret: "configured", PublicURL: "http://localhost", Issuer: "https://issuer.test", Store: ls, Sessions: sessions})
	if err != nil {
		t.Fatal(err)
	}
	l := &routeLimiter{}
	verifier := &credential.Verifier{Store: drainStore{}, Audience: "http://localhost/mcp"}
	return newWebMux(webDeps{store: routeAccountStore{}, sessions: sessions, login: login, verifier: verifier, tools: mcpapi.Config{Limiter: l}, loginLimit: newLoginLimiter(time.Now, nil)}), l, ls
}
func TestRoutes_CredentialsNeverCross(t *testing.T) {
	mux, _, _ := routeFixture(t)
	for _, tt := range protectedRoutes() {
		t.Run(tt.method+tt.path, func(t *testing.T) {
			for _, bearer := range []bool{false, true} {
				r := httptest.NewRequest(tt.method, tt.path, nil)
				if bearer {
					r.Header.Set("Authorization", "Bearer trc_"+strings.Repeat("A", 43))
				}
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, r)
				if w.Code != 401 {
					t.Fatalf("bearer=%t status=%d", bearer, w.Code)
				}
			}
		})
	}
	valid := httptest.NewRequest("POST", "/mcp", nil)
	valid.Header.Set("Authorization", "Bearer trc_"+strings.Repeat("A", 43))
	validResponse := httptest.NewRecorder()
	mux.ServeHTTP(validResponse, valid)
	if validResponse.Code == http.StatusUnauthorized {
		t.Fatal("valid bearer rejected by MCP")
	}
	r := httptest.NewRequest("POST", "/mcp", nil)
	r.AddCookie(&http.Cookie{Name: "__Host-session", Value: strings.Repeat("A", 43)})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("session authorized MCP: %d", w.Code)
	}
}
func TestRoutes_CSRFAndBuckets(t *testing.T) {
	for _, tt := range protectedRoutes() {
		t.Run(tt.method+tt.path, func(t *testing.T) {
			mux, l, _ := routeFixture(t)
			request := func(csrf bool) *httptest.ResponseRecorder {
				r := httptest.NewRequest(tt.method, tt.path, nil)
				r.AddCookie(&http.Cookie{Name: "__Host-session", Value: strings.Repeat("A", 43)})
				if csrf {
					r.Header.Set("Origin", "http://localhost")
					r.Header.Set("X-CSRF-Token", strings.Repeat("A", 43))
				}
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, r)
				return w
			}
			if tt.method != "GET" {
				w := request(false)
				if w.Code != 403 || len(l.calls) != 0 {
					t.Fatalf("missing CSRF: status=%d calls=%d", w.Code, len(l.calls))
				}
			}
			w := request(true)
			if w.Code != 429 || len(l.calls) != 1 || l.calls[0] != tt.class {
				t.Fatalf("status=%d buckets=%v", w.Code, l.calls)
			}
		})
	}
}
func TestRoutes_LoginContract(t *testing.T) {
	mux, _, ls := routeFixture(t)
	for _, bearer := range []bool{false, true} {
		r := httptest.NewRequest("GET", "/api/v1/auth/google/start", nil)
		if bearer {
			r.Header.Set("Authorization", "Bearer trc_"+strings.Repeat("A", 43))
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != 302 || len(w.Result().Cookies()) == 0 {
			t.Fatalf("start status=%d", w.Code)
		}
	}
	for _, cookie := range []bool{false, true} {
		r := httptest.NewRequest("GET", "/api/v1/auth/google/callback", nil)
		if cookie {
			r.AddCookie(&http.Cookie{Name: "__Host-login", Value: strings.Repeat("A", 43)})
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != 302 || w.Header().Get("Location") != "/login?error=login_expired" {
			t.Fatalf("callback status=%d", w.Code)
		}
	}
	if ls.claims != 1 {
		t.Fatalf("claim calls=%d", ls.claims)
	}
}

func TestRoutes_LoginSharesIPBudget(t *testing.T) {
	mux, _, _ := routeFixture(t)
	for i := 0; i < 21; i++ {
		path := "/api/v1/auth/google/start"
		if i%2 != 0 {
			path = "/api/v1/auth/google/callback"
		}
		r := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		want := http.StatusFound
		if i == 20 {
			want = http.StatusTooManyRequests
		}
		if w.Code != want {
			t.Fatalf("request %d status=%d, want %d", i+1, w.Code, want)
		}
	}
}

func TestRoutes_OAuthMounted(t *testing.T) {
	for _, tt := range []struct{ method, path string }{
		{"GET", "/.well-known/oauth-protected-resource"}, {"GET", "/.well-known/oauth-protected-resource/mcp"}, {"GET", "/.well-known/oauth-authorization-server"},
		{"GET", "/oauth/authorize"}, {"GET", "/oauth/consent/invalid"}, {"POST", "/oauth/consent/invalid"}, {"POST", "/oauth/token"}, {"POST", "/oauth/register"}, {"POST", "/oauth/revoke"},
		{"GET", "/api/v1/me/grants"}, {"DELETE", "/api/v1/me/grants/1"},
	} {
		t.Run(tt.method+tt.path, func(t *testing.T) {
			mux, _, _ := routeFixture(t)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, nil))
			if w.Code == http.StatusNotFound || w.Code == http.StatusMethodNotAllowed {
				t.Fatalf("route missing: %d", w.Code)
			}
		})
	}
}

func TestRoutes_OAuthSharesLoginBudget(t *testing.T) {
	mux, _, _ := routeFixture(t)
	for i := range 21 {
		path := "/.well-known/oauth-authorization-server"
		if i%2 == 0 {
			path = "/api/v1/auth/google/start"
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if i == 20 && w.Code != http.StatusTooManyRequests {
			t.Fatalf("request %d status=%d", i+1, w.Code)
		}
		if i < 20 && w.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d limited early", i+1)
		}
	}
}

func TestRoutes_OAuthCredentials(t *testing.T) {
	for _, credential := range []string{"none", "session", "bearer"} {
		t.Run(credential, func(t *testing.T) {
			mux, _, _ := routeFixture(t)
			r := httptest.NewRequest("POST", "/oauth/token", nil)
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if credential == "session" {
				r.AddCookie(&http.Cookie{Name: "__Host-session", Value: strings.Repeat("A", 43)})
			}
			if credential == "bearer" {
				r.Header.Set("Authorization", "Bearer trc_"+strings.Repeat("A", 43))
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			want := http.StatusBadRequest
			if credential == "bearer" {
				want = http.StatusUnauthorized
			}
			if w.Code != want {
				t.Fatalf("status=%d want=%d", w.Code, want)
			}
		})
	}
}

func TestRoutes_OAuthRateLimitJSON(t *testing.T) {
	for _, path := range []string{"/oauth/token", "/oauth/register", "/oauth/revoke"} {
		t.Run(path, func(t *testing.T) {
			mux, _, _ := routeFixture(t)
			for range 20 {
				mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/.well-known/oauth-authorization-server", nil))
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest("POST", path, nil))
			if w.Code != 429 || w.Header().Get("Retry-After") != "3" || w.Header().Get("Content-Type") != "application/json" || strings.TrimSpace(w.Body.String()) != `{"error":"temporarily_unavailable"}` {
				t.Fatalf("status=%d headers=%v body=%s", w.Code, w.Header(), w.Body.String())
			}
		})
	}
}

func TestRoutes_OAuthRateLimitPage(t *testing.T) {
	for _, tt := range []struct{ method, path, lang, message string }{
		{"GET", "/oauth/authorize", "vi", "Dịch vụ tạm thời không khả dụng"},
		{"GET", "/oauth/consent/handle?lang=en", "en", "The service is temporarily unavailable"},
		{"POST", "/oauth/consent/handle", "vi", "Dịch vụ tạm thời không khả dụng"},
	} {
		t.Run(tt.method+tt.path, func(t *testing.T) {
			mux, _, _ := routeFixture(t)
			for range 20 {
				mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/.well-known/oauth-authorization-server", nil))
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, nil))
			h := w.Header()
			body := w.Body.String()
			if w.Code != 429 || h.Get("Retry-After") != "3" || h.Get("Content-Type") != "text/html; charset=utf-8" || h.Get("Cache-Control") != "no-store" || h.Get("X-Frame-Options") != "DENY" || h.Get("Referrer-Policy") != "same-origin" || !strings.Contains(h.Get("Content-Security-Policy"), "default-src 'none'; style-src 'self' 'sha256-") || !strings.Contains(h.Get("Content-Security-Policy"), "frame-ancestors 'none'") || !strings.Contains(body, `<html lang="`+tt.lang+`">`) || !strings.Contains(body, tt.message) || !strings.Contains(body, "temporarily_unavailable") || strings.Contains(body, "<a ") || h.Get("Location") != "" {
				t.Fatalf("status=%d headers=%v body=%s", w.Code, h, body)
			}
		})
	}
}
