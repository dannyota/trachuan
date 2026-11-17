package app

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/mcpapi"
	"danny.vn/trachuan/apps/server/internal/weblogin"
	"danny.vn/trachuan/apps/server/internal/websession"
)

func publicLimitFixture() http.Handler {
	clock := func() time.Time { return time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC) }
	return newWebMux(webDeps{
		store:      routeAccountStore{},
		sessions:   &websession.Manager{Store: routeSessionStore{}, PublicURL: "http://localhost"},
		login:      &weblogin.Handler{},
		verifier:   &credential.Verifier{},
		tools:      mcpapi.Config{Limiter: &routeLimiter{}, Resolver: publicTaxonomyResolver{engine: &publicTaxonomyEngine{}}, Clock: clock},
		loginLimit: newLoginLimiter(clock, []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128")}),
		publicURL:  "http://localhost",
	})
}

func TestPublicLimit_RoutesPeerAndForwarding(t *testing.T) {
	paths := []string{"/api/v1/public/status", "/api/v1/public/taxonomy", "/api/v1/public/taxonomy/1", "/api/v1/public/taxonomy/codes"}
	for _, peer := range []struct {
		addr     string
		loopback bool
	}{
		{"127.0.0.1:1234", true},
		{"127.0.0.2:1234", true},
		{"[::1]:1234", true},
		{"[::ffff:127.0.0.1]:1234", true},
		{"192.0.2.1:1234", false},
		{"[2001:db8::1]:1234", false},
		{"127.0.0.1", false},
		{"localhost:1234", false},
	} {
		for _, forwarded := range []struct {
			name   string
			values []string
		}{
			{"absent", nil},
			{"external", []string{"198.51.100.1"}},
			{"loopback", []string{"127.0.0.1"}},
			{"ipv6_loopback", []string{"::1"}},
			{"empty", []string{""}},
			{"list", []string{"198.51.100.1, 127.0.0.1"}},
			{"multiple", []string{"198.51.100.1", "127.0.0.1"}},
			{"invalid", []string{"invalid"}},
		} {
			t.Run(peer.addr+"/"+forwarded.name, func(t *testing.T) {
				mux := publicLimitFixture()
				for i := range 128 {
					path := paths[i%len(paths)]
					r := httptest.NewRequest(http.MethodGet, path, nil)
					r.RemoteAddr = peer.addr
					if forwarded.values != nil {
						r.Header["X-Forwarded-For"] = forwarded.values
					}
					w := httptest.NewRecorder()
					mux.ServeHTTP(w, r)
					want := http.StatusOK
					if i >= 60 && (!peer.loopback || forwarded.values != nil) {
						want = http.StatusTooManyRequests
					}
					if w.Code != want {
						t.Fatalf("%s request %d: status=%d want=%d body=%s", path, i+1, w.Code, want, w.Body.String())
					}
					if want == http.StatusTooManyRequests && w.Header().Get("Retry-After") != "1" {
						t.Fatalf("Retry-After=%q want=1", w.Header().Get("Retry-After"))
					}
				}
			})
		}
	}
}

func TestPublicLimit_BypassIndependentOfBucket(t *testing.T) {
	for _, peer := range []string{"127.0.0.1:1234", "[::1]:1234"} {
		t.Run(peer, func(t *testing.T) {
			mux := publicLimitFixture()
			call := func(forwarded bool) int {
				r := httptest.NewRequest(http.MethodGet, "/api/v1/public/status", nil)
				r.RemoteAddr = peer
				if forwarded {
					r.Header.Set("X-Forwarded-For", "")
				}
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, r)
				return w.Code
			}
			for range 60 {
				if got := call(false); got != http.StatusOK {
					t.Fatalf("local request: status=%d", got)
				}
				if got := call(true); got != http.StatusOK {
					t.Fatalf("forwarded request: status=%d", got)
				}
			}
			if got := call(true); got != http.StatusTooManyRequests {
				t.Fatalf("exhausted bucket: status=%d", got)
			}
			if got := call(false); got != http.StatusOK {
				t.Fatalf("local request with exhausted bucket: status=%d", got)
			}
		})
	}
}

func TestPublicLimit_OtherRoutesKeepBuckets(t *testing.T) {
	for _, peer := range []string{"127.0.0.1:1234", "[::1]:1234"} {
		for _, route := range []struct {
			method, path string
			budget, want int
		}{
			{http.MethodPost, "/api/v1/auth/password", 20, http.StatusServiceUnavailable},
			{http.MethodPost, "/api/v1/auth/signup", 5, http.StatusServiceUnavailable},
			{http.MethodPost, "/api/v1/auth/reset", 5, http.StatusServiceUnavailable},
			{http.MethodGet, "/.well-known/oauth-authorization-server", 20, http.StatusOK},
			{http.MethodPost, "/oauth/token", 20, http.StatusBadRequest},
			{http.MethodGet, "/oauth/authorize", 20, http.StatusBadRequest},
		} {
			t.Run(peer+route.path, func(t *testing.T) {
				mux := publicLimitFixture()
				for i := range route.budget + 1 {
					r := httptest.NewRequest(route.method, route.path, nil)
					r.RemoteAddr = peer
					w := httptest.NewRecorder()
					mux.ServeHTTP(w, r)
					want := route.want
					if i == route.budget {
						want = http.StatusTooManyRequests
					}
					if w.Code != want {
						t.Fatalf("request %d: status=%d want=%d body=%s", i+1, w.Code, want, w.Body.String())
					}
				}
			})
		}
	}
}
