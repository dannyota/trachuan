package websession

import (
	"net/http"
	"testing"
)

func TestSessionProbe_InvalidAndUnavailable(t *testing.T) {
	for _, tc := range []struct {
		name, raw, path string
		unavailable     bool
		status          int
		clear           bool
	}{
		{"no cookie", "", "/api/v1/me", false, 204, false},
		{"malformed", "bad", "/api/v1/me", false, 204, true},
		{"unknown", rawValue(8), "/api/v1/me", false, 204, true},
		{"unavailable", rawValue(1), "/api/v1/me", true, 503, false},
		{"other route", "bad", "/api/v1/status", false, 401, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _, _ := harness()
			if tc.unavailable {
				m.Store = nil
			}
			r := request(tc.raw)
			r.URL.Path = tc.path
			w := serve(m, r)
			assertStatus(t, w, tc.status)
			cookies := w.Result().Cookies()
			if tc.clear {
				if len(cookies) != 1 {
					t.Fatalf("cookies=%v", cookies)
				}
				c := cookies[0]
				if c.Name != "__Host-session" || c.Value != "" || c.Path != "/" || c.Domain != "" || !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.MaxAge >= 0 {
					t.Fatalf("cookie=%+v", c)
				}
			} else if len(cookies) != 0 {
				t.Fatalf("cookies=%v", cookies)
			}
			if tc.status == 204 && w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("missing no-store")
			}
		})
	}
}
