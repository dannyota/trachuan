package websession

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
)

func TestGoogle_CallbackCookieDoesNotAuthorizePendingConsumption(t *testing.T) {
	db := &pendingStore{}
	m := &Manager{Store: db}
	r := httptest.NewRequest("GET", "/api/v1/auth/google/callback?pending_hash="+hashToken(strings.Repeat("A", 43)), nil)
	r.AddCookie(&http.Cookie{Name: "__Host-pending", Value: strings.Repeat("A", 43)})
	var required *store.PendingRequired
	if err := m.CreateGoogleSession(t.Context(), httptest.NewRecorder(), r, store.GoogleIdentity{Next: "/"}); !errors.As(err, &required) {
		t.Fatal("primary did not stage factor authentication", err)
	}
	if len(db.params.ReplacedHashes) != 0 {
		t.Fatal("callback input authorized pending consumption")
	}
}

type googleSessionSuccess struct{ pendingStore }

func (s *googleSessionSuccess) CreateGoogleSession(_ context.Context, p store.GoogleSessionParams) (store.CreateSessionRow, error) {
	s.params = p.Pending
	now := time.Now()
	return store.CreateSessionRow{CreatedAt: now, ExpiresAt: now.Add(time.Hour)}, nil
}
func TestGoogle_BoundPendingClearedWithoutCallbackCookie(t *testing.T) {
	db := &googleSessionSuccess{}
	m := &Manager{Store: db}
	r := httptest.NewRequest("GET", "/api/v1/auth/google/callback", nil)
	old := hashToken(strings.Repeat("A", 43))
	w := httptest.NewRecorder()
	if err := m.CreateGoogleSession(t.Context(), w, r, store.GoogleIdentity{PendingHash: old}); err != nil {
		t.Fatal(err)
	}
	if len(db.params.ReplacedHashes) != 1 || db.params.ReplacedHashes[0] != old {
		t.Fatal("start binding lost before sign-in transaction")
	}
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == "__Host-pending" && cookie.MaxAge < 0 {
			return
		}
	}
	t.Fatal("pending cookie not cleared after sign-in")
}
