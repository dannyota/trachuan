package app

import (
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestPublicStatus_MountedAndLimited(t *testing.T) {
	mux, limiter, _ := routeFixture(t)
	for i := 0; i <= 60; i++ {
		r := httptest.NewRequest("GET", "/api/v1/public/status", nil)
		r.Header.Set("Authorization", "Bearer invalid")
		r.Header.Set("Cookie", "__Host-session=invalid")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if i < 60 && (w.Code != 503 || !strings.Contains(w.Body.String(), `"code":"unavailable"`)) {
			t.Fatalf("request %d: %d %s", i, w.Code, w.Body.String())
		}
		if i == 60 && (w.Code != 429 || w.Header().Get("Retry-After") == "" || !strings.Contains(w.Body.String(), `"code":"rate_limited"`)) {
			t.Fatalf("request %d: %d %s", i, w.Code, w.Body.String())
		}
	}
	if len(limiter.calls) != 0 {
		t.Fatal("public route used account limiter")
	}
}

func TestPublicStatus_TrustedProxyAndSeparateLoginBucket(t *testing.T) {
	mux, _, _ := routeFixture(t, netip.MustParsePrefix("10.0.0.0/8"))
	call := func(path, peer, forwarded string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.RemoteAddr = peer + ":1234"
		r.Header.Set("X-Forwarded-For", forwarded)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	for i := 0; i < 60; i++ {
		if w := call("/api/v1/public/status", "10.0.0.1", "192.0.2.1"); w.Code != 503 {
			t.Fatal(i, w.Code)
		}
	}
	if w := call("/api/v1/public/status", "10.0.0.2", "192.0.2.1"); w.Code != 429 {
		t.Fatal(w.Code)
	}
	if w := call("/api/v1/public/status", "10.0.0.1", "192.0.2.2"); w.Code != 503 {
		t.Fatal(w.Code)
	}
	if w := call("/api/v1/auth/google/start", "10.0.0.1", "192.0.2.1"); w.Code == 429 {
		t.Fatal("public bucket exhausted login bucket")
	}
	for i := 0; i < 60; i++ {
		if w := call("/api/v1/public/status", "198.51.100.1", "192.0.2.1"); w.Code != 503 {
			t.Fatal(i, w.Code)
		}
	}
	if w := call("/api/v1/public/status", "198.51.100.1", "192.0.2.2"); w.Code != 429 {
		t.Fatal("untrusted forwarded address used", w.Code)
	}
}
