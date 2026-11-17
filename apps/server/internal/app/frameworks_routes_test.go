package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/mcpapi"
	"danny.vn/trachuan/apps/server/internal/weblogin"
	"danny.vn/trachuan/apps/server/internal/websession"
	"github.com/stretchr/testify/require"
)

func frameworkPathRoutes(enabled bool) (http.Handler, *routeLimiter) {
	limiter := &routeLimiter{}
	sessions := &websession.Manager{Store: routeSessionStore{}, PublicURL: "http://localhost"}
	return newWebMux(webDeps{store: routeAccountStore{}, sessions: sessions, login: &weblogin.Handler{}, tools: mcpapi.Config{Frameworks: enabled, Limiter: limiter}, loginLimit: newLoginLimiter(time.Now, nil), publicURL: sessions.PublicURL}), limiter
}

func TestFrameworkRoutes_MalformedPathsNeverRedirect(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, path := range []string{
			"/api/v1/frameworks/controls/..",
			"/api/v1/frameworks/controls/.",
			"/api/v1/frameworks/versions/..",
			"/api/v1/frameworks/versions/.",
			"/api/v1/frameworks/controls/%2e%2e",
			"/api/v1/frameworks/controls/%2E",
			"/api/v1/frameworks//controls/fw:fw-alpha:1%23A",
			"/api/v1/frameworks/controls//fw:fw-alpha:1%23A",
			"/api/v1/frameworks/controls/fw:fw-alpha:1%23A%2FB",
			"/api/v1/frameworks/controls%2ffw:fw-alpha:1%23A",
			"/api/v1/frameworks%2Fcontrols/fw:fw-alpha:1%23A",
			"/api/v1/frameworks/controls/%2F..",
			"/api/v1/frameworks/../status",
			"/api/v1/frameworks/./search",
		} {
			for _, method := range []string{"GET", "POST", "CONNECT"} {
				t.Run(fmt.Sprintf("%t/%s%s", enabled, method, path), func(t *testing.T) {
					h, limiter := frameworkPathRoutes(enabled)
					r := httptest.NewRequest(method, path, nil)
					r.AddCookie(&http.Cookie{Name: "__Host-session", Value: strings.Repeat("A", 43)})
					w := httptest.NewRecorder()
					h.ServeHTTP(w, r)
					want, code := 404, "not_found"
					if enabled {
						want, code = 400, "invalid_request"
						if strings.HasPrefix(r.URL.Path, "/api/v1/frameworks/controls/") || strings.HasPrefix(r.URL.Path, "/api/v1/frameworks/versions/") {
							code = "invalid_handle"
						}
					}
					require.Equal(t, want, w.Code)
					require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
					require.Empty(t, w.Header().Get("Location"))
					require.Contains(t, w.Body.String(), `"code":"`+code+`"`)
					require.Empty(t, limiter.calls)
				})
			}
		}
	}
}

func TestFrameworkRoutes_CanonicalPathsKeepSessionGuards(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, path := range []string{
			"/api/v1/frameworks",
			"/api/v1/frameworks/controls/fw:fw-alpha:1%23A%252FB",
			"/api/v1/frameworks/versions/fw:fw-alpha:1",
			"/api/v1/frameworks/mappings",
			"/api/v1/frameworks/gaps",
		} {
			t.Run(fmt.Sprintf("%t/%s", enabled, path), func(t *testing.T) {
				h, _ := frameworkPathRoutes(enabled)
				w := httptest.NewRecorder()
				h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
				want := 404
				if enabled {
					want = 401
				}
				require.Equal(t, want, w.Code)
				require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			})
		}
	}
	for _, path := range []string{"/api/v1/frameworks", "/api/v1/frameworks/controls/fw:fw-alpha:1%23A%252FB"} {
		h, limiter := frameworkPathRoutes(true)
		r := httptest.NewRequest("GET", path, nil)
		r.AddCookie(&http.Cookie{Name: "__Host-session", Value: strings.Repeat("A", 43)})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		require.Equal(t, 429, w.Code)
		require.Len(t, limiter.calls, 1)
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	}
}
