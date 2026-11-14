package weblogin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/store"
)

type capturedGoogle struct{ identity store.GoogleIdentity }

func (s *capturedGoogle) CreateGoogleSession(_ context.Context, _ http.ResponseWriter, _ *http.Request, id store.GoogleIdentity) error {
	s.identity = id
	return nil
}

func TestGoogle_PendingBindingComesOnlyFromLoginStart(t *testing.T) {
	for _, scenario := range []string{"bound", "absent", "malformed", "duplicate"} {
		t.Run(scenario, func(t *testing.T) {
			f := newFixture(t)
			captured := &capturedGoogle{}
			f.h.sessions = captured
			raw := strings.Repeat("A", 43)
			sum := sha256.Sum256([]byte(raw))
			hash := hex.EncodeToString(sum[:])
			start := httptest.NewRequest("GET", startPath+"?next=/&pending_hash="+strings.Repeat("b", 64), nil)
			switch scenario {
			case "bound":
				start.AddCookie(&http.Cookie{Name: "__Host-pending", Value: raw})
			case "malformed":
				start.AddCookie(&http.Cookie{Name: "__Host-pending", Value: "invalid"})
			case "duplicate":
				start.AddCookie(&http.Cookie{Name: "__Host-pending", Value: raw})
				start.AddCookie(&http.Cookie{Name: "__Host-pending", Value: raw})
			}
			w := httptest.NewRecorder()
			f.h.Start.ServeHTTP(w, start)
			if w.Code != 302 {
				t.Fatal("start failed")
			}
			location := w.Header().Get("Location")
			if strings.Contains(location, raw) || strings.Contains(location, hash) {
				t.Fatal("pending binding exposed to provider")
			}
			u, err := url.Parse(location)
			if err != nil {
				t.Fatal(err)
			}
			a := attempt{cookie: cookieNamed(t, w, "__Host-login"), state: u.Query().Get("state"), nonce: u.Query().Get("nonce"), challenge: u.Query().Get("code_challenge")}
			f.register(a, "synthetic-code", nil)
			callback := httptest.NewRequest("GET", callbackPath+"?"+url.Values{"state": {a.state}, "code": {"synthetic-code"}, "pending_hash": {strings.Repeat("b", 64)}}.Encode(), nil)
			callback.AddCookie(a.cookie)
			callback.AddCookie(&http.Cookie{Name: "__Host-pending", Value: strings.Repeat("B", 43)})
			w = httptest.NewRecorder()
			f.h.Callback.ServeHTTP(w, callback)
			if w.Code != 302 || w.Header().Get("Location") != "/" {
				t.Fatal("callback failed")
			}
			want := ""
			if scenario == "bound" {
				want = hash
			}
			if captured.identity.PendingHash != want {
				t.Fatal("pending binding did not come from login start")
			}
		})
	}
}
