package app

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/mcpapi"
	"danny.vn/trachuan/apps/server/internal/weblogin"
	"danny.vn/trachuan/apps/server/internal/websession"
	"golang.org/x/oauth2"
)

func originRouteFixture(t *testing.T, mode string, origins []string, clock func() time.Time, logger *slog.Logger) (http.Handler, *slowBodyAuth) {
	t.Helper()
	if clock == nil {
		clock = time.Now
	}
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	sessions := &websession.Manager{Store: routeSessionStore{}, PublicURL: "https://example.test"}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{Transport: discoveryRoundTrip{}})
	login, err := weblogin.New(ctx, weblogin.Config{ClientID: "id", ClientSecret: "configured", PublicURL: sessions.PublicURL, Issuer: "https://issuer.test", Store: &routeLoginStore{}, Sessions: sessions})
	if err != nil {
		t.Fatal(err)
	}
	auth := &slowBodyAuth{}
	return newWebMux(webDeps{store: routeAccountStore{}, sessions: sessions, login: login, verifier: &credential.Verifier{Store: auth, Audience: sessions.PublicURL + "/mcp"}, tools: mcpapi.Config{Limiter: &routeLimiter{}, Logger: logger}, loginLimit: newLoginLimiter(clock, nil), publicURL: sessions.PublicURL, mcpOrigins: origins, mcpOriginMode: mode}), auth
}

func assertNoCORS(t *testing.T, h http.Header) {
	t.Helper()
	for key := range h {
		if strings.HasPrefix(strings.ToLower(key), "access-control-") {
			t.Fatalf("CORS header %s present", key)
		}
	}
}

func assertOriginForbidden(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	cacheControl := w.Header().Get("Cache-Control")
	if w.Code != http.StatusForbidden || w.Header().Get("Content-Type") != "application/json" || (cacheControl != "no-store" && cacheControl != mcpapi.CacheControl) || w.Body.String() != `{"error":"invalid_request"}` {
		t.Fatalf("status=%d headers=%v body=%s", w.Code, w.Header(), w.Body.String())
	}
	assertNoCORS(t, w.Header())
}

func TestMCPOrigin_HeaderPolicy(t *testing.T) {
	for _, mode := range []string{"log", "enforce"} {
		for _, tc := range []struct {
			name    string
			origins []string
			outside bool
		}{
			{name: "absent"},
			{name: "public", origins: []string{"https://example.test"}},
			{name: "listed", origins: []string{"https://CLIENT.test:443"}},
			{name: "unlisted", origins: []string{"https://outside.test"}, outside: true},
			{name: "null", origins: []string{"null"}, outside: true},
			{name: "two allowed", origins: []string{"https://example.test", "https://CLIENT.test:443"}, outside: true},
			{name: "duplicate allowed", origins: []string{"https://example.test", "https://example.test"}, outside: true},
			{name: "folded", origins: []string{"https://example.test, https://CLIENT.test:443"}, outside: true},
			{name: "empty", origins: []string{""}, outside: true},
			{name: "case differs", origins: []string{"https://client.test:443"}, outside: true},
			{name: "port differs", origins: []string{"https://CLIENT.test"}, outside: true},
			{name: "trailing slash", origins: []string{"https://example.test/"}, outside: true},
			{name: "spaces", origins: []string{" https://example.test "}, outside: true},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				mux, _ := originRouteFixture(t, mode, []string{"https://CLIENT.test:443"}, nil, nil)
				r := httptest.NewRequest("GET", "/mcp", nil)
				r.Header["Origin"] = tc.origins
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, r)
				if mode == "enforce" && tc.outside {
					assertOriginForbidden(t, w)
				} else if w.Code != http.StatusUnauthorized || w.Header().Get("WWW-Authenticate") == "" {
					t.Fatalf("request did not proceed unchanged: status=%d headers=%v", w.Code, w.Header())
				}
				assertNoCORS(t, w.Header())
			})
		}
	}
}

func TestMCPOrigin_DefaultEnforces(t *testing.T) {
	mux, _ := originRouteFixture(t, "", nil, nil, nil)
	r := httptest.NewRequest("GET", "/mcp", nil)
	r.Header.Set("Origin", "https://outside.test")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	assertOriginForbidden(t, w)
}

func TestMCPOrigin_RejectedRequestsPreserveIPBucket(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		allowedCode  int
	}{
		{"GET", "/.well-known/oauth-protected-resource", 200},
		{"GET", "/.well-known/oauth-protected-resource/mcp", 200},
		{"GET", "/.well-known/oauth-authorization-server", 200},
		{"POST", "/oauth/token", 400},
		{"POST", "/oauth/register", 400},
		{"POST", "/oauth/revoke", 400},
	} {
		t.Run(tc.path, func(t *testing.T) {
			now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
			mux, _ := originRouteFixture(t, "enforce", nil, func() time.Time { return now }, nil)
			call := func(origin string) *httptest.ResponseRecorder {
				r := httptest.NewRequest(tc.method, tc.path, nil)
				r.Header.Set("Origin", origin)
				r.Header.Set("Content-Type", "application/json")
				if tc.path != "/oauth/register" {
					r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				}
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, r)
				return w
			}
			for range 32 {
				assertOriginForbidden(t, call("https://outside.test"))
			}
			for i := range 21 {
				want := tc.allowedCode
				if i == 20 {
					want = http.StatusTooManyRequests
				}
				if w := call("https://example.test"); w.Code != want {
					t.Fatalf("allowed request %d: status=%d want=%d", i+1, w.Code, want)
				}
			}
			assertOriginForbidden(t, call("https://outside.test"))
		})
	}
}

func TestMCPOrigin_RouteCoverage(t *testing.T) {
	for _, tc := range []struct{ method, path string }{
		{"GET", "/mcp"}, {"POST", "/mcp"}, {"PUT", "/mcp"}, {"DELETE", "/mcp"}, {"PATCH", "/mcp"}, {"HEAD", "/mcp"}, {"OPTIONS", "/mcp"}, {"CUSTOM", "/mcp"},
		{"POST", "/oauth/token"}, {"POST", "/oauth/register"}, {"POST", "/oauth/revoke"},
		{"GET", "/.well-known/oauth-protected-resource/mcp"}, {"GET", "/.well-known/oauth-protected-resource"}, {"GET", "/.well-known/oauth-authorization-server"},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			mux, auth := originRouteFixture(t, "enforce", nil, nil, nil)
			r := httptest.NewRequest(tc.method, tc.path, nil)
			r.Header.Set("Origin", "https://outside.test")
			r.Header.Set("Authorization", "Bearer trc_"+strings.Repeat("A", 43))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			assertOriginForbidden(t, w)
			if auth.calls.Load() != 0 || w.Header().Get("WWW-Authenticate") != "" {
				t.Fatal("origin check ran after credential verification")
			}
		})
	}
}

func TestMCPOrigin_OtherRoutesUnchanged(t *testing.T) {
	for _, tc := range []struct{ method, path string }{
		{"GET", "/oauth/authorize"}, {"GET", "/oauth/consent/invalid"}, {"POST", "/oauth/consent/invalid"},
		{"GET", "/api/v1/me"}, {"POST", "/api/v1/search"}, {"GET", "/api/v1/public/status"},
		{"GET", "/api/v1/auth/google/start"}, {"GET", "/login"}, {"GET", "/oauth/token"}, {"POST", "/.well-known/oauth-authorization-server"},
		{"HEAD", "/.well-known/oauth-protected-resource"}, {"HEAD", "/.well-known/oauth-protected-resource/mcp"}, {"HEAD", "/.well-known/oauth-authorization-server"},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			var baseline *httptest.ResponseRecorder
			for _, mode := range []string{"log", "enforce"} {
				var logs bytes.Buffer
				mux, _ := originRouteFixture(t, mode, nil, nil, slog.New(slog.NewJSONHandler(&logs, nil)))
				r := httptest.NewRequest(tc.method, tc.path, nil)
				r.Header.Set("Origin", "https://outside.test")
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, r)
				if baseline == nil {
					baseline = w
				} else if w.Code != baseline.Code || (w.Code != http.StatusFound && w.Body.String() != baseline.Body.String()) || w.Header().Get("Content-Type") != baseline.Header().Get("Content-Type") {
					t.Fatalf("route changed in enforce mode: status=%d want=%d", w.Code, baseline.Code)
				}
				assertNoCORS(t, w.Header())
				if logs.Len() != 0 {
					t.Fatal("route observed by MCP origin policy")
				}
			}
		})
	}
}

func TestMCPOrigin_ClientResponsesNoCORS(t *testing.T) {
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/.well-known/oauth-protected-resource", "", 200},
		{"GET", "/.well-known/oauth-protected-resource/mcp", "", 200},
		{"GET", "/.well-known/oauth-authorization-server", "", 200},
		{"POST", "/oauth/token", "", 400},
		{"POST", "/oauth/register", "{}", 400},
		{"POST", "/oauth/revoke", "", 400},
		{"POST", "/mcp", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"origin-test","version":"1"}}}`, 200},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			mux, _ := originRouteFixture(t, "log", nil, nil, nil)
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			r.Header.Set("Origin", "https://outside.test")
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if tc.path == "/oauth/register" || tc.path == "/mcp" {
				r.Header.Set("Content-Type", "application/json")
			}
			if tc.path == "/mcp" {
				r.Header.Set("Authorization", "Bearer trc_"+strings.Repeat("A", 43))
				r.Header.Set("Accept", "application/json, text/event-stream")
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", w.Code, tc.status, w.Body.String())
			}
			assertNoCORS(t, w.Header())
		})
	}
}
