package websession

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"

	"danny.vn/trachuan/apps/server/internal/contracttest"
)

func testLogoutContract(t *testing.T, coverage *contracttest.Coverage) {
	m, _, raw := harness()
	r := request(raw)
	r.Method = "POST"
	r.Header.Set("Origin", "https://example.invalid")
	r.Header.Set("X-CSRF-Token", rawValue(2))
	w := httptest.NewRecorder()
	m.Middleware(m.CSRF(http.HandlerFunc(m.Logout))).ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatalf("status %d", w.Code)
	}
	coverage.Check(t, "POST", "/api/v1/auth/logout", w)
}

func testLogoutContract_Errors(t *testing.T, coverage *contracttest.Coverage) {
	for _, status := range []int{401, 403, 503} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			m, s, raw := harness()
			r := request(raw)
			r.Method = "POST"
			r.Header.Set("Origin", "https://example.invalid")
			r.Header.Set("X-CSRF-Token", rawValue(2))
			if status == 401 {
				r.Header.Del("Cookie")
			}
			if status == 403 {
				r.Header.Del("X-CSRF-Token")
			}
			if status == 503 {
				s.fail = "revoke"
			}
			w := httptest.NewRecorder()
			m.Middleware(m.CSRF(http.HandlerFunc(m.Logout))).ServeHTTP(w, r)
			if w.Code != status {
				t.Fatalf("status %d", w.Code)
			}
			coverage.Check(t, "POST", "/api/v1/auth/logout", w)
		})
	}
}

func TestOpenAPIContract(t *testing.T) {
	c, err := contracttest.Load()
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for path := range c.Paths {
		if path == "/api/v1/auth/logout" {
			paths = append(paths, path)
		}
	}
	coverage := contracttest.NewCoverage(c, paths)
	t.Run("TestLogoutContract", func(t *testing.T) { testLogoutContract(t, coverage) })
	t.Run("TestLogoutContract_Errors", func(t *testing.T) { testLogoutContract_Errors(t, coverage) })
	// The application mux contract test exercises throttling, which wraps these handlers.
	want := []string{"post /api/v1/auth/logout 429 application/json"}
	if missing := coverage.Missing(); !slices.Equal(missing, want) {
		t.Errorf("unexercised responses: %v", missing)
	}
}
