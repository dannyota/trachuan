package webapi

import (
	"net/http/httptest"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/contracttest"

	"danny.vn/trachuan/apps/server/internal/release"
)

func testWebContract(t *testing.T, coverage *contracttest.Coverage) {
	cases := []struct{ method, path, actual, body string }{
		{"POST", "/api/v1/search", "/api/v1/search", `{"country":"vn","query":"law"}`}, {"POST", "/api/v1/read", "/api/v1/read", `{"country":"vn","items":[{"handle":"vn:1#a"}]}`}, {"GET", "/api/v1/documents/{key}", "/api/v1/documents/vn:1", ""}, {"GET", "/api/v1/status", "/api/v1/status", ""}}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			w := webCall(newHarness(t), tc.method, tc.actual, tc.body)
			if w.Code != 200 {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			coverage.Check(t, tc.method, tc.path, w)
		})
	}
}

func testWebContract_Guards(t *testing.T, coverage *contracttest.Coverage) {
	cases := []struct{ method, path, actual string }{
		{"POST", "/api/v1/search", "/api/v1/search"}, {"POST", "/api/v1/read", "/api/v1/read"}, {"GET", "/api/v1/documents/{key}", "/api/v1/documents/vn:1"}, {"GET", "/api/v1/status", "/api/v1/status"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			h := newHarness(t)
			r := webRequest(tc.method, tc.actual, "")
			r.Header.Del("Cookie")
			w := httptest.NewRecorder()
			h.web.ServeHTTP(w, r)
			if w.Code != 401 {
				t.Fatalf("status %d", w.Code)
			}
			coverage.Check(t, tc.method, tc.path, w)
			if tc.method == "POST" {
				r = webRequest(tc.method, tc.actual, "")
				r.Header.Del("X-CSRF-Token")
				w = httptest.NewRecorder()
				h.web.ServeHTTP(w, r)
				if w.Code != 403 {
					t.Fatalf("status %d", w.Code)
				}
				coverage.Check(t, tc.method, tc.path, w)
			}
		})
	}
}

func testWebContract_Errors(t *testing.T, coverage *contracttest.Coverage) {
	cases := []struct{ method, path, actual, body string }{
		{"POST", "/api/v1/search", "/api/v1/search", `{"country":"vn","query":"law"}`}, {"POST", "/api/v1/read", "/api/v1/read", `{"country":"vn","items":[]}`}, {"GET", "/api/v1/documents/{key}", "/api/v1/documents/vn:1", ""}, {"GET", "/api/v1/status", "/api/v1/status", ""},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			h := newHarness(t)
			h.resolver.err = release.ErrExpired
			w := webCall(h, tc.method, tc.actual, tc.body)
			if w.Code != 409 {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			coverage.Check(t, tc.method, tc.path, w)
			bad := webCall(newHarness(t), tc.method, tc.actual+"?other=x", "invalid")
			if tc.method == "POST" {
				bad = webCall(newHarness(t), tc.method, tc.actual, "invalid")
			}
			if bad.Code != 400 {
				t.Fatalf("status %d: %s", bad.Code, bad.Body.String())
			}
			coverage.Check(t, tc.method, tc.path, bad)
			unavailable := httptest.NewRecorder()
			New(Config{}).ServeHTTP(unavailable, httptest.NewRequest(tc.method, tc.actual, nil))
			if unavailable.Code != 503 {
				t.Fatal(unavailable.Code)
			}
			coverage.Check(t, tc.method, tc.path, unavailable)
		})
	}
}

func testWebContract_BodyAndRateLimits(t *testing.T, coverage *contracttest.Coverage) {
	for _, tc := range []struct {
		method, path, actual, body string
		count                      int
	}{
		{"POST", "/api/v1/search", "/api/v1/search", `{"country":"vn","query":"law"}`, 60},
		{"POST", "/api/v1/read", "/api/v1/read", `{"country":"vn","items":[]}`, 120},
		{"GET", "/api/v1/documents/{key}", "/api/v1/documents/vn:1", "", 120},
		{"GET", "/api/v1/status", "/api/v1/status", "", 120},
	} {
		t.Run(tc.path, func(t *testing.T) {
			if tc.method == "POST" {
				w := webCall(newHarness(t), tc.method, tc.actual, strings.Repeat("x", (64<<10)+1))
				if w.Code != 413 {
					t.Fatal(w.Code)
				}
				coverage.Check(t, tc.method, tc.path, w)
			}
			h := newHarness(t)
			var w *httptest.ResponseRecorder
			for i := 0; i <= tc.count; i++ {
				w = webCall(h, tc.method, tc.actual, tc.body)
			}
			if w.Code != 429 || w.Header().Get("Retry-After") == "" {
				t.Fatalf("status %d headers %v", w.Code, w.Header())
			}
			coverage.Check(t, tc.method, tc.path, w)
		})
	}
}

func TestOpenAPIContract(t *testing.T) {
	c, err := contracttest.Load()
	if err != nil {
		t.Fatal(err)
	}
	coverage := contracttest.NewCoverageForTag(c, "web")
	t.Run("TestWebContract", func(t *testing.T) { testWebContract(t, coverage) })
	t.Run("TestWebContract_Guards", func(t *testing.T) { testWebContract_Guards(t, coverage) })
	t.Run("TestWebContract_Errors", func(t *testing.T) { testWebContract_Errors(t, coverage) })
	t.Run("TestWebContract_BodyAndRateLimits", func(t *testing.T) { testWebContract_BodyAndRateLimits(t, coverage) })
	t.Run("populated", func(t *testing.T) { testWebContract_Populated(t, coverage) })
	t.Run("request boundaries", func(t *testing.T) { testRequestContractBoundaries(t, c) })
	if missing := coverage.Missing(); len(missing) > 0 {
		t.Errorf("unexercised responses: %v", missing)
	}
}

func testWebContract_Populated(t *testing.T, coverage *contracttest.Coverage) {
	h := newHarness(t)
	h.resolver.engine = &populatedEngine{testEngine: h.engine}
	for _, tc := range []struct {
		method, path, body string
		fields             []string
	}{
		{"POST", "/api/v1/search", `{"country":"vn","query":"ngân hàng","detail":"full"}`, []string{`"documents"`, `"fragments"`, `"relations"`}},
		{"POST", "/api/v1/search", `{"country":"vn","query":"ngân hàng","detail":"compact"}`, []string{`"documents"`, `"snippet"`}},
		{"POST", "/api/v1/read", `{"country":"vn","items":[{"handle":"vn:1"}]}`, []string{`"part"`, `"parts"`, `"next"`}},
		{"GET", "/api/v1/documents/vn:1", "", []string{`"document"`, `"part"`, `"next"`}},
	} {
		w := webCall(h, tc.method, tc.path, tc.body)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
		contractPath := tc.path
		if tc.method == "GET" {
			contractPath = "/api/v1/documents/{key}"
		}
		coverage.Check(t, tc.method, contractPath, w)
		for _, field := range tc.fields {
			if !strings.Contains(w.Body.String(), field) {
				t.Errorf("%s missing %s", tc.path, field)
			}
		}
	}
}
