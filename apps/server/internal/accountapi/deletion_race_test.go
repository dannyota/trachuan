package accountapi

import (
	"context"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
)

type deletionRaceStore struct {
	*fakeStore
	before func()
}

func (s *deletionRaceStore) RequestDeletion(ctx context.Context, id int64) (store.RequestDeletionRow, error) {
	s.before()
	return s.fakeStore.RequestDeletion(ctx, id)
}
func TestDelete_CredentialsCreatedAfterEnumeration(t *testing.T) {
	_, s, v, raw, secret := setup(t)
	m := &websession.Manager{Store: s, Clock: func() time.Time { return s.now }, PublicURL: "https://example.invalid"}
	session := "eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHg"
	token, digest, err := credential.Issue(strings.NewReader(strings.Repeat("x", 32)))
	if err != nil {
		t.Fatal(err)
	}
	probe := m.Middleware(New(Config{Store: s, Sessions: m, Verifier: v, Limiter: limits.New(func() time.Time { return s.now })}))
	db := &deletionRaceStore{fakeStore: s, before: func() {
		s.sessions = append(s.sessions, store.WebSession{ID: 3, AccountID: 1, TokenHash: hash(session), CsrfSecret: secret, CreatedAt: s.now, LastSeenAt: s.now, ExpiresAt: s.now.Add(time.Hour)})
		s.tokens = append(s.tokens, store.PersonalToken{ID: 1, AccountID: 1, TokenHash: digest, ExpiresAt: s.now.Add(time.Hour)})
		expect(t, call(probe, session, secret, "GET", "/api/v1/me", ""), 200)
		if _, err := v.Verify(context.Background(), token); err != nil {
			t.Fatal(err)
		}
	}}
	h := m.Middleware(m.CSRF(New(Config{Store: db, Sessions: m, Verifier: v, Limiter: limits.New(func() time.Time { return s.now })})))
	expect(t, call(h, raw, secret, "DELETE", "/api/v1/me", ""), 204)
	expect(t, call(probe, session, secret, "GET", "/api/v1/me", ""), 401)
	if _, err := v.Verify(context.Background(), token); err == nil {
		t.Fatal("new token remains cached")
	}
}
