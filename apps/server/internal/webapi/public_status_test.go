package webapi

import (
	"bytes"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/apierror"
	"danny.vn/trachuan/apps/server/internal/contracttest"
	"danny.vn/trachuan/apps/server/internal/mcpapi"
	"danny.vn/trachuan/apps/server/internal/release"
)

func testPublicStatus(t *testing.T, coverage *contracttest.Coverage) {
	h := newHarness(t)
	public := NewPublicStatus(h.webConfig())
	want := webCall(h, "GET", "/api/v1/status", "")
	for _, credential := range []string{"", "cookie", "bearer"} {
		r := httptest.NewRequest("GET", "/api/v1/public/status", nil)
		if credential == "cookie" {
			r.Header.Set("Cookie", "__Host-session=invalid")
		}
		if credential == "bearer" {
			r.Header.Set("Authorization", "Bearer invalid")
		}
		w := httptest.NewRecorder()
		public.ServeHTTP(w, r)
		if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), want.Body.Bytes()) || w.Header().Get("Cache-Control") != "public, max-age=600" {
			t.Fatalf("credential %s: %d %s", credential, w.Code, w.Body.String())
		}
		coverage.Check(t, "GET", "/api/v1/public/status", w)
	}
	for _, tc := range []struct {
		path   string
		fail   bool
		status int
		code   string
	}{
		{"/api/v1/public/status?bad=1", false, 400, "invalid_request"},
		{"/api/v1/public/status", true, 503, "unavailable"},
	} {
		if tc.fail {
			h.resolver.err = errors.New("unavailable")
		}
		w := httptest.NewRecorder()
		public.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != tc.status || !strings.Contains(w.Body.String(), `"code":"`+tc.code+`"`) {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		coverage.Check(t, "GET", "/api/v1/public/status", w)
	}
	h.resolver.err = release.ErrExpired
	expired := httptest.NewRecorder()
	public.ServeHTTP(expired, httptest.NewRequest("GET", "/api/v1/public/status", nil))
	if expired.Code != 503 || !strings.Contains(expired.Body.String(), `"code":"unavailable"`) {
		t.Fatal(expired.Code, expired.Body.String())
	}
	h.resolver.err = nil
	h.engine.err = errors.New("corpus unavailable")
	w := httptest.NewRecorder()
	public.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/public/status", nil))
	if w.Code != 503 || !strings.Contains(w.Body.String(), `"code":"unavailable"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	apierror.RateLimited(w, time.Second)
	coverage.Check(t, "GET", "/api/v1/public/status", w)
}

func (h *harness) webConfig() mcpapi.Config { return mcpapi.Config{Resolver: h.resolver} }
