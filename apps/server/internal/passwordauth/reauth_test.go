package passwordauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/password"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
)

type proofStore struct {
	err   error
	calls int
}

func (*proofStore) GetAccount(context.Context, int64) (store.Account, error) {
	return store.Account{ID: 1, Email: "person@example.test"}, nil
}
func (s *proofStore) ReauthPassword(context.Context, store.PasswordSnapshot, int64) error {
	s.calls++
	return s.err
}

type sessionStore struct{ websession.Store }

func (sessionStore) GetSession(context.Context, string) (store.GetSessionRow, error) {
	now := time.Now()
	return store.GetSessionRow{ID: 2, AccountID: 1, CsrfSecret: strings.Repeat("A", 43), CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(time.Hour)}, nil
}
func reauthFixture(h *Handler) http.Handler {
	sessions := &websession.Manager{Store: sessionStore{}, PublicURL: "https://example.test"}
	return sessions.Middleware(sessions.CSRF(http.HandlerFunc(h.Reauth)))
}
func reauthRequest(body string) *http.Request {
	r := httptest.NewRequest("POST", "/api/v1/me/reauth", strings.NewReader(body))
	r.AddCookie(&http.Cookie{Name: "__Host-session", Value: strings.Repeat("A", 43)})
	r.Header.Set("Origin", "https://example.test")
	r.Header.Set("X-CSRF-Token", strings.Repeat("A", 43))
	r.Header.Set("Content-Type", "application/json")
	return r
}
func TestReauth_ProofAndSharedCounter(t *testing.T) {
	hash := "synthetic"
	version := int64(1)
	for _, scenario := range []string{"success", "wrong", "passwordless", "stale authority", "capped", "busy"} {
		t.Run(scenario, func(t *testing.T) {
			db := fakeStore{row: store.PasswordSnapshot{AccountID: 1, Hash: &hash, Version: &version}}
			hasher := &fakeHasher{match: true}
			proofs := &proofStore{}
			counter := password.NewCounter(time.Now)
			switch scenario {
			case "wrong":
				hasher.match = false
			case "passwordless":
				db.row.Hash = nil
			case "stale authority":
				proofs.err = store.ErrLoginFailed
			case "capped":
				for range 10 {
					counter.Allow("person@example.test")
				}
			case "busy":
				hasher.err = password.ErrBusy
			}
			h := &Handler{Store: db, Hasher: hasher, Counter: counter, Proofs: proofs}
			w := httptest.NewRecorder()
			reauthFixture(h).ServeHTTP(w, reauthRequest(`{"password":"synthetic passphrase"}`))
			want := 401
			if scenario == "success" {
				want = 204
			}
			if scenario == "busy" {
				want = 503
			}
			if w.Code != want {
				t.Fatalf("got %d want %d", w.Code, want)
			}
			if w.Code == 401 && !strings.Contains(w.Body.String(), "reauth_failed") {
				t.Fatal("wrong error code")
			}
			if len(w.Result().Cookies()) != 0 {
				t.Fatal("reauth replaced cookie")
			}
		})
	}
}
