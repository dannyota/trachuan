package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/contracttest"
)

func TestPasswordRoutes_UseLoginBucketAndSessionCSRF(t *testing.T) {
	mux, _, _ := routeFixture(t)
	for range 20 {
		mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/v1/auth/google/start", nil))
	}
	r := httptest.NewRequest("POST", "/api/v1/auth/password", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	c, err := contracttest.Load()
	if err != nil {
		t.Fatal(err)
	}
	contracttest.NewCoverageForTag(c, "password").Check(t, "POST", "/api/v1/auth/password", w)
	if w.Code != 429 {
		t.Fatalf("password route not sharing login bucket: %d", w.Code)
	}
	r = httptest.NewRequest("POST", "/api/v1/me/reauth", strings.NewReader(`{}`))
	r.AddCookie(&http.Cookie{Name: "__Host-session", Value: strings.Repeat("A", 43)})
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("reauth missing CSRF: %d", w.Code)
	}
}
