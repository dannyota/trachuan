package websession

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCSRF_Matrix(t *testing.T) {
	tests := []struct {
		name, method, origin, referer, token, contentType, body string
		want                                                    int
	}{
		{"same origin", "POST", "https://example.invalid", "", rawValue(2), "application/json", "{}", 204},
		{"different origin", "POST", "https://evil.invalid", "", rawValue(2), "application/json", "{}", 403},
		{"null", "POST", "null", "https://example.invalid/", rawValue(2), "application/json", "{}", 403},
		{"referer fallback", "POST", "", "https://example.invalid/account", rawValue(2), "application/json", "{}", 204},
		{"absent origin evidence", "POST", "", "", rawValue(2), "application/json", "{}", 403},
		{"foreign referer", "POST", "", "https://evil.invalid/", rawValue(2), "application/json", "{}", 403},
		{"missing token", "POST", "https://example.invalid", "", "", "application/json", "{}", 403},
		{"wrong token", "POST", "https://example.invalid", "", rawValue(3), "application/json", "{}", 403},
		{"non JSON body", "POST", "https://example.invalid", "", rawValue(2), "text/plain", "{}", 403},
		{"missing media type", "POST", "https://example.invalid", "", rawValue(2), "", "{}", 403},
		{"charset", "POST", "https://example.invalid", "", rawValue(2), "application/json; charset=utf-8", "{}", 204},
		{"bodiless logout", "POST", "https://example.invalid", "", rawValue(2), "", "", 204},
		{"bodiless delete", "DELETE", "https://example.invalid", "", rawValue(2), "", "", 204},
		{"patch enforced", "PATCH", "https://evil.invalid", "", rawValue(2), "application/json", "{}", 403},
		{"trailing slash origin", "POST", "https://example.invalid/", "", rawValue(2), "application/json", "{}", 403},
		{"suffix host", "POST", "https://example.invalid.evil.invalid", "", rawValue(2), "application/json", "{}", 403},
		{"port differs", "POST", "https://example.invalid:444", "", rawValue(2), "application/json", "{}", 403},
		{"referer userinfo", "POST", "", "https://attacker@example.invalid/", rawValue(2), "application/json", "{}", 403},
		{"referer malformed", "POST", "", "://broken", rawValue(2), "application/json", "{}", 403},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, _, raw := harness()
			r := httptest.NewRequest(tc.method, "/api/v1/read", strings.NewReader(tc.body))
			r.AddCookie(&http.Cookie{Name: "__Host-session", Value: raw})
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if tc.referer != "" {
				r.Header.Set("Referer", tc.referer)
			}
			r.Header.Set("X-CSRF-Token", tc.token)
			if tc.contentType != "" {
				r.Header.Set("Content-Type", tc.contentType)
			}
			w := httptest.NewRecorder()
			m.Middleware(m.CSRF(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))).ServeHTTP(w, r)
			assertStatus(t, w, tc.want)
			if tc.want == 403 {
				assertCSRFError(t, w)
			}
		})
	}
	for _, method := range []string{"GET", "HEAD", "OPTIONS"} {
		t.Run(method, func(t *testing.T) {
			m, _, _ := harness()
			w := httptest.NewRecorder()
			m.CSRF(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })).ServeHTTP(w, httptest.NewRequest(method, "/api/v1/me", nil))
			assertStatus(t, w, 204)
		})
	}
	for _, tc := range []struct {
		name, header string
		values       []string
		fallback     bool
	}{
		{"duplicate token", "X-CSRF-Token", []string{rawValue(2), rawValue(2)}, false},
		{"folded token", "X-CSRF-Token", []string{rawValue(2) + ", " + rawValue(2)}, false},
		{"duplicate origin", "Origin", []string{"https://example.invalid", "https://example.invalid"}, false},
		{"empty origin", "Origin", []string{""}, false},
		{"folded origin", "Origin", []string{"https://example.invalid, https://example.invalid"}, false},
		{"duplicate referer", "Referer", []string{"https://example.invalid/", "https://example.invalid/"}, true},
		{"folded referer", "Referer", []string{"https://example.invalid/a, https://evil.invalid/"}, true},
		{"duplicate media type", "Content-Type", []string{"application/json", "application/json"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _, _ := harness()
			r := httptest.NewRequest("POST", "/api/v1/read", strings.NewReader("{}"))
			r.Header.Set("Origin", "https://example.invalid")
			r.Header.Set("Referer", "https://example.invalid/")
			r.Header.Set("X-CSRF-Token", rawValue(2))
			r.Header.Set("Content-Type", "application/json")
			if tc.fallback {
				r.Header.Del("Origin")
			}
			r.Header.Del(tc.header)
			for _, value := range tc.values {
				r.Header.Add(tc.header, value)
			}
			r = r.WithContext(context.WithValue(r.Context(), principalKey{}, Principal{CSRFSecret: rawValue(2)}))
			w := httptest.NewRecorder()
			m.CSRF(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })).ServeHTTP(w, r)
			assertStatus(t, w, 403)
			assertCSRFError(t, w)
		})
	}
	t.Run("chunked body", func(t *testing.T) {
		m, _, _ := harness()
		r := httptest.NewRequest("POST", "/api/v1/read", nil)
		r.TransferEncoding = []string{"chunked"}
		r.Header.Set("Origin", "https://example.invalid")
		r.Header.Set("X-CSRF-Token", rawValue(2))
		r = r.WithContext(context.WithValue(r.Context(), principalKey{}, Principal{CSRFSecret: rawValue(2)}))
		w := httptest.NewRecorder()
		m.CSRF(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })).ServeHTTP(w, r)
		assertStatus(t, w, 403)
	})
	t.Run("no principal", func(t *testing.T) {
		m, _, _ := harness()
		r := httptest.NewRequest("POST", "/api/v1/read", nil)
		r.Header.Set("Origin", "https://example.invalid")
		r.Header.Set("X-CSRF-Token", rawValue(2))
		w := httptest.NewRecorder()
		m.CSRF(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })).ServeHTTP(w, r)
		assertStatus(t, w, 403)
	})
}

func assertCSRFError(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Body.String() != "{\"error\":{\"code\":\"csrf_rejected\",\"message\":\"CSRF validation failed\"}}\n" {
		t.Fatalf("error=%s", w.Body.String())
	}
}

func TestCSRF_NormalizedOrigins(t *testing.T) {
	tests := []struct {
		name, publicURL, header, value string
		want                           int
	}{
		{"https public port", "https://example.invalid:443/base", "Origin", "https://example.invalid", 204},
		{"https origin port", "https://example.invalid/base", "Origin", "https://example.invalid:443", 204},
		{"http public port", "http://example.invalid:80/base", "Origin", "http://example.invalid", 204},
		{"http origin port", "http://example.invalid/base", "Origin", "http://example.invalid:80", 204},
		{"host and scheme case", "HTTPS://EXAMPLE.INVALID/base", "Origin", "https://example.invalid", 204},
		{"origin host case", "https://example.invalid/base", "Origin", "https://EXAMPLE.INVALID", 204},
		{"referer normalized", "https://EXAMPLE.INVALID:443/base", "Referer", "https://example.invalid/account?x=1", 204},
		{"referer default port", "http://example.invalid/base", "Referer", "http://EXAMPLE.INVALID:80/account", 204},
		{"ipv6 default port", "https://[::1]:443/base", "Origin", "https://[::1]", 204},
		{"different port", "https://example.invalid:443", "Origin", "https://example.invalid:444", 403},
		{"different scheme", "https://example.invalid", "Origin", "http://example.invalid", 403},
		{"origin with path", "https://example.invalid", "Origin", "https://example.invalid/path", 403},
		{"origin with slash", "https://example.invalid", "Origin", "https://example.invalid/", 403},
		{"origin with query", "https://example.invalid", "Origin", "https://example.invalid?x=1", 403},
		{"origin with empty query", "https://example.invalid", "Origin", "https://example.invalid?", 403},
		{"origin with fragment", "https://example.invalid", "Origin", "https://example.invalid#", 403},
		{"origin with userinfo", "https://example.invalid", "Origin", "https://u@example.invalid", 403},
		{"origin empty port", "https://example.invalid", "Origin", "https://example.invalid:", 403},
		{"origin invalid port", "https://example.invalid", "Origin", "https://example.invalid:65536", 403},
		{"malformed referer", "https://example.invalid", "Referer", "https://example.invalid:bad/a", 403},
		{"referer userinfo", "https://example.invalid", "Referer", "https://u@example.invalid/a", 403},
		{"referer fragment", "https://example.invalid", "Referer", "https://example.invalid/a#part", 403},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, _, raw := harness()
			m.PublicURL = tc.publicURL
			r := httptest.NewRequest("POST", "/api/v1/read", nil)
			r.AddCookie(&http.Cookie{Name: "__Host-session", Value: raw})
			r.Header.Set("X-CSRF-Token", rawValue(2))
			r.Header.Set(tc.header, tc.value)
			w := httptest.NewRecorder()
			m.Middleware(m.CSRF(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))).ServeHTTP(w, r)
			assertStatus(t, w, tc.want)
		})
	}
}
