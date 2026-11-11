package websession

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLogout_RevokesAndClears(t *testing.T) {
	m, _, raw := harness()
	assertStatus(t, serve(m, request(raw)), 204)
	r := request(raw)
	r.Method = "POST"
	r.Header.Set("Origin", "https://example.invalid")
	r.Header.Set("X-CSRF-Token", rawValue(2))
	w := httptest.NewRecorder()
	m.Middleware(m.CSRF(http.HandlerFunc(m.Logout))).ServeHTTP(w, r)
	assertStatus(t, w, 204)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Value != "" || cookies[0].MaxAge != -1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].Path != "/" || cookies[0].SameSite != http.SameSiteLaxMode || w.Header().Get("Clear-Site-Data") != `"cookies"` {
		t.Fatal("logout did not clear browser state")
	}
	assertStatus(t, serve(m, request(raw)), 401)
	t.Run("revoke failure", func(t *testing.T) {
		m, s, raw := harness()
		s.fail = "revoke"
		w := httptest.NewRecorder()
		r := request(raw)
		r.Method = http.MethodPost
		m.Logout(w, r)
		assertStatus(t, w, 503)
		if len(w.Result().Cookies()) != 0 {
			t.Fatal("reported logout on failure")
		}
	})
}
