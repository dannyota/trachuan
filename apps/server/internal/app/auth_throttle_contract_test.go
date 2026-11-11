package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/contracttest"
)

func TestRoutes_AuthThrottleContract(t *testing.T) {
	c, err := contracttest.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/v1/auth/google/start"},
		{"GET", "/api/v1/auth/google/callback"},
		{"POST", "/api/v1/auth/logout"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			schema := c.Paths[tc.path][strings.ToLower(tc.method)].Responses["429"].Headers["Retry-After"].Schema
			if schema["type"] != "integer" {
				t.Fatal("429 must document Retry-After")
			}
			mux, _, _ := routeFixture(t)
			if tc.method == "GET" {
				for range 20 {
					mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", tc.path, nil))
				}
			}
			r := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.method == "POST" {
				r.AddCookie(&http.Cookie{Name: "__Host-session", Value: strings.Repeat("A", 43)})
				r.Header.Set("Origin", "http://localhost")
				r.Header.Set("X-CSRF-Token", strings.Repeat("A", 43))
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != 429 || w.Header().Get("Retry-After") == "" {
				t.Fatalf("status=%d retry=%q", w.Code, w.Header().Get("Retry-After"))
			}
			contracttest.NewCoverage(c, []string{tc.path}).Check(t, tc.method, tc.path, w)
		})
	}
}
