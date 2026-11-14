package app

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFactorRoutes_RequirePendingOrSession(t *testing.T) {
	mux, _, _ := routeFixture(t)
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/api/v1/auth/factor", 401},
		{"POST", "/api/v1/auth/factor", 403},
		{"POST", "/api/v1/me/totp/setup", 404},
		{"POST", "/api/v1/me/totp/confirm", 404},
		{"DELETE", "/api/v1/me/totp", 401},
		{"POST", "/api/v1/me/recovery-codes", 401},
	} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`)))
		if w.Code != tc.status {
			t.Errorf("%s %s: got %d want %d", tc.method, tc.path, w.Code, tc.status)
		}
	}
}
