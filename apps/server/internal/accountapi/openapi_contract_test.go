package accountapi

import (
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/contracttest"

	"danny.vn/trachuan/apps/server/internal/websession"
)

func testAccountContract(t *testing.T, coverage *contracttest.Coverage) {
	cases := []struct{ method, path, actual, body string }{
		{"GET", "/api/v1/me", "/api/v1/me", ""}, {"DELETE", "/api/v1/me", "/api/v1/me", ""},
		{"GET", "/api/v1/me/sessions", "/api/v1/me/sessions", ""}, {"DELETE", "/api/v1/me/sessions/{id}", "/api/v1/me/sessions/1", ""},
		{"GET", "/api/v1/me/tokens", "/api/v1/me/tokens", ""}, {"POST", "/api/v1/me/tokens", "/api/v1/me/tokens", `{"name":"test"}`}, {"DELETE", "/api/v1/me/tokens/{id}", "/api/v1/me/tokens/9", ""}}
	for _, tc := range cases {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			h, _, _, raw, secret := setup(t)
			w := call(h, raw, secret, tc.method, tc.actual, tc.body)
			want := map[string]int{"GET /api/v1/me": 200, "DELETE /api/v1/me": 204, "GET /api/v1/me/sessions": 200, "DELETE /api/v1/me/sessions/{id}": 204, "GET /api/v1/me/tokens": 200, "POST /api/v1/me/tokens": 201, "DELETE /api/v1/me/tokens/{id}": 404}[tc.method+" "+tc.path]
			if w.Code != want {
				t.Fatalf("status %d, want %d", w.Code, want)
			}
			coverage.Check(t, tc.method, tc.path, w)
		})
	}
}

func testAccountContract_Guards(t *testing.T, coverage *contracttest.Coverage) {
	cases := []struct{ method, path, actual string }{
		{"GET", "/api/v1/me", "/api/v1/me"}, {"DELETE", "/api/v1/me", "/api/v1/me"},
		{"GET", "/api/v1/me/sessions", "/api/v1/me/sessions"}, {"DELETE", "/api/v1/me/sessions/{id}", "/api/v1/me/sessions/1"},
		{"GET", "/api/v1/me/tokens", "/api/v1/me/tokens"}, {"POST", "/api/v1/me/tokens", "/api/v1/me/tokens"}, {"DELETE", "/api/v1/me/tokens/{id}", "/api/v1/me/tokens/1"},
	}
	for _, tc := range cases {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			h, _, _, raw, _ := setup(t)
			w := call(h, "invalid", "", tc.method, tc.actual, "")
			if w.Code != 401 {
				t.Fatalf("status %d", w.Code)
			}
			coverage.Check(t, tc.method, tc.path, w)
			if tc.method != "GET" {
				w = call(h, raw, "invalid", tc.method, tc.actual, "")
				if w.Code != 403 {
					t.Fatalf("status %d", w.Code)
				}
				coverage.Check(t, tc.method, tc.path, w)
			}
		})
	}
}

func testAccountContract_Errors(t *testing.T, coverage *contracttest.Coverage) {
	cases := []struct{ method, path, actual, body string }{
		{"GET", "/api/v1/me", "/api/v1/me", ""}, {"DELETE", "/api/v1/me", "/api/v1/me", ""},
		{"GET", "/api/v1/me/sessions", "/api/v1/me/sessions", ""}, {"DELETE", "/api/v1/me/sessions/{id}", "/api/v1/me/sessions/1", ""},
		{"GET", "/api/v1/me/tokens", "/api/v1/me/tokens", ""}, {"POST", "/api/v1/me/tokens", "/api/v1/me/tokens", `{"name":"test"}`}, {"DELETE", "/api/v1/me/tokens/{id}", "/api/v1/me/tokens/1", ""},
	}
	for _, tc := range cases {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			h, s, _, raw, secret := setup(t)
			for range 120 {
				w := call(h, raw, secret, "GET", "/api/v1/me", "")
				if w.Code != 200 {
					t.Fatal(w.Code)
				}
			}
			w := call(h, raw, secret, tc.method, tc.actual, tc.body)
			if w.Code != 429 || w.Header().Get("Retry-After") == "" {
				t.Fatalf("status %d", w.Code)
			}
			coverage.Check(t, tc.method, tc.path, w)
			m := &websession.Manager{Store: s, Clock: func() time.Time { return s.now }, PublicURL: "https://example.invalid"}
			bad := m.Middleware(m.CSRF(New(Config{})))
			w = call(bad, raw, secret, tc.method, tc.actual, tc.body)
			if w.Code != 503 {
				t.Fatalf("status %d", w.Code)
			}
			coverage.Check(t, tc.method, tc.path, w)
		})
	}
	for _, tc := range []struct{ path, actual string }{{"/api/v1/me/sessions/{id}", "/api/v1/me/sessions/99"}, {"/api/v1/me/tokens/{id}", "/api/v1/me/tokens/99"}} {
		h, _, _, raw, secret := setup(t)
		w := call(h, raw, secret, "DELETE", tc.actual, "")
		if w.Code != 404 {
			t.Fatal(w.Code)
		}
		coverage.Check(t, "DELETE", tc.path, w)
	}
	for _, tc := range []struct {
		body   string
		status int
	}{{"invalid", 400}, {`{"name":"` + strings.Repeat("x", (64<<10)+1) + `"}`, 413}} {
		h, _, _, raw, secret := setup(t)
		w := call(h, raw, secret, "POST", "/api/v1/me/tokens", tc.body)
		if w.Code != tc.status {
			t.Fatal(w.Code)
		}
		coverage.Check(t, "POST", "/api/v1/me/tokens", w)
	}
	h, _, _, raw, secret := setup(t)
	call(h, raw, secret, "POST", "/api/v1/me/tokens", `{"name":"test"}`)
	w := call(h, raw, secret, "DELETE", "/api/v1/me/tokens/1", "")
	if w.Code != 204 {
		t.Fatal(w.Code)
	}
	coverage.Check(t, "DELETE", "/api/v1/me/tokens/{id}", w)
}

func TestOpenAPIContract(t *testing.T) {
	c, err := contracttest.Load()
	if err != nil {
		t.Fatal(err)
	}
	coverage := contracttest.NewCoverageForTag(c, "account")
	t.Run("TestAccountContract", func(t *testing.T) { testAccountContract(t, coverage) })
	t.Run("TestAccountContract_Guards", func(t *testing.T) { testAccountContract_Guards(t, coverage) })
	t.Run("TestAccountContract_Errors", func(t *testing.T) { testAccountContract_Errors(t, coverage) })
	t.Run("grants", func(t *testing.T) { testGrantsContract(t, coverage) })
	t.Run("token limit", func(t *testing.T) { testTokensLimitContract(t, coverage) })
	t.Run("request boundaries", func(t *testing.T) { testTokenRequestContractDays(t, c) })
	if missing := coverage.Missing(); len(missing) > 0 {
		t.Errorf("unexercised responses: %v", missing)
	}
}
