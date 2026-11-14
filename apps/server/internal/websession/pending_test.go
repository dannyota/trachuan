package websession

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/store"
)

type pendingStore struct {
	Store
	params  store.PendingParams
	session store.CreateSessionParams
	err     error
}

func (s *pendingStore) CreatePasswordSession(_ context.Context, p store.PasswordSessionParams) (store.CreateSessionRow, error) {
	s.params, s.session = p.Pending, p.Session
	if s.err != nil {
		return store.CreateSessionRow{}, s.err
	}
	return store.CreateSessionRow{}, &store.PendingRequired{CSRF: p.Session.CsrfSecret}
}
func (s *pendingStore) CreateGoogleSession(_ context.Context, p store.GoogleSessionParams) (store.CreateSessionRow, error) {
	s.params, s.session = p.Pending, p.Session
	if s.err != nil {
		return store.CreateSessionRow{}, s.err
	}
	return store.CreateSessionRow{}, &store.PendingRequired{CSRF: p.Session.CsrfSecret}
}
func TestPending_PrimaryCookiesAndBrowserReplacement(t *testing.T) {
	for _, google := range []bool{false, true} {
		for _, next := range []string{"/oauth/consent/synthetic", "//hostile.test"} {
			db := &pendingStore{}
			m := &Manager{Store: db}
			r := httptest.NewRequest("POST", "/", nil)
			old := strings.Repeat("A", 43)
			r.AddCookie(&http.Cookie{Name: "__Host-pending", Value: old})
			w := httptest.NewRecorder()
			var err error
			if google {
				err = m.CreateGoogleSession(t.Context(), w, r, store.GoogleIdentity{Next: next, PendingHash: hashToken(old)})
			} else {
				err = m.CreatePasswordLogin(t.Context(), w, r, store.PasswordSnapshot{AccountID: 42}, next)
			}
			var pending *store.PendingRequired
			if !errors.As(err, &pending) || pending.CSRF != db.session.CsrfSecret || !validValue(pending.CSRF) {
				t.Fatal("pending result missing", err)
			}
			cookies := w.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatal("primary issued session")
			}
			cookie := cookies[0]
			if cookie.Name != "__Host-pending" || cookie.Path != "/" || !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.MaxAge != 300 || hashToken(cookie.Value) != db.session.TokenHash || !validValue(cookie.Value) {
				t.Fatal("pending cookie policy")
			}
			want := next
			if next == "//hostile.test" {
				want = "/"
			}
			if db.params.Next != want || len(db.params.ReplacedHashes) != 1 || db.params.ReplacedHashes[0] != hashToken(old) {
				t.Fatal("pending destination or browser binding lost")
			}
		}
	}
}
func TestPending_RejectsDuplicateCookiesAndClearsExpired(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: "__Host-pending", Value: strings.Repeat("A", 43)})
	if _, ok := PendingToken(r); !ok {
		t.Fatal("valid cookie refused")
	}
	r.AddCookie(&http.Cookie{Name: "__Host-pending", Value: strings.Repeat("A", 43)})
	if _, ok := PendingToken(r); ok {
		t.Fatal("duplicate cookies accepted")
	}
	w := httptest.NewRecorder()
	ClearPending(w)
	cookie := w.Result().Cookies()[0]
	if cookie.MaxAge != -1 || cookie.Name != "__Host-pending" || !cookie.Secure || !cookie.HttpOnly || cookie.Path != "/" || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("clear cookie policy")
	}
}
