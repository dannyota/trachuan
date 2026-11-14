package websession

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"danny.vn/trachuan/apps/server/internal/store"
)

func TestCommitSession_AmbiguousCommitEvictsAndSetsNoCookie(t *testing.T) {
	m, s, raw := harness()
	assertStatus(t, serve(m, request(raw)), 204)
	w := httptest.NewRecorder()
	err := m.CommitSession(t.Context(), w, request(raw), func(context.Context, store.CreateSessionParams) (store.CreateSessionRow, error) {
		s.mu.Lock()
		old := s.rows[hashValue(raw)]
		old.RevokedAt = &s.now
		s.rows[hashValue(raw)] = old
		s.mu.Unlock()
		return store.CreateSessionRow{AccountID: 42}, errors.New("ambiguous commit")
	})
	if err == nil || len(w.Result().Cookies()) != 0 {
		t.Fatal("uncertain commit set cookie")
	}
	assertStatus(t, serve(m, request(raw)), 401)
}

func TestCommitSession_UncertainRegistrationEvictsOtherAccounts(t *testing.T) {
	m, s, presented := harness()
	probe := func(raw string) int {
		recorder := httptest.NewRecorder()
		m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })).ServeHTTP(recorder, request(raw))
		return recorder.Code
	}
	revoked := rawValue(3)
	other := s.rows[hashValue(presented)]
	other.ID, other.AccountID, other.TokenHash = 8, 44, hashValue(revoked)
	s.rows[other.TokenHash] = other
	for _, raw := range []string{presented, revoked} {
		if probe(raw) != 204 {
			t.Fatal("session did not warm cache")
		}
	}
	w := httptest.NewRecorder()
	err := m.CommitSession(t.Context(), w, request(presented), func(_ context.Context, p store.CreateSessionParams) (store.CreateSessionRow, error) {
		if len(p.ReplacedHashes) != 1 || p.ReplacedHashes[0] != hashValue(presented) {
			t.Fatal("presented session not replaced")
		}
		s.mu.Lock()
		for _, raw := range []string{presented, revoked} {
			row := s.rows[hashValue(raw)]
			row.RevokedAt = &s.now
			s.rows[hashValue(raw)] = row
		}
		s.mu.Unlock()
		return store.CreateSessionRow{AccountID: 43, RevokedHashes: []string{hashValue(revoked)}}, errors.New("commit acknowledgement failed")
	})
	if err == nil || len(w.Result().Cookies()) != 0 {
		t.Fatal("uncertain registration issued cookie")
	}
	for _, raw := range []string{presented, revoked} {
		if got := probe(raw); got != 401 {
			t.Errorf("revoked session stayed cached: status=%d", got)
		}
	}
}
