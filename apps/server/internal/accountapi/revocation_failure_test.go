package accountapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
)

type uncertainStore struct {
	*fakeStore
	cancel context.CancelFunc
	fail   bool
}

func (s *uncertainStore) outcome() error {
	if s.cancel != nil {
		s.cancel()
	}
	if s.fail {
		return errors.New("database outcome unknown")
	}
	return nil
}
func (s *uncertainStore) RevokeOwnToken(ctx context.Context, p store.RevokeOwnTokenParams) (string, error) {
	n, err := s.fakeStore.RevokeOwnToken(ctx, p)
	if err != nil {
		return n, err
	}
	return n, s.outcome()
}
func (s *uncertainStore) RequestDeletion(ctx context.Context, id int64) (store.RequestDeletionRow, error) {
	row, err := s.fakeStore.RequestDeletion(ctx, id)
	if err != nil {
		return row, err
	}
	return row, s.outcome()
}

func TestRevocation_UncertainOutcomeEvictsCredentials(t *testing.T) {
	for _, route := range []string{"token", "deletion"} {
		for _, outcome := range []string{"canceled", "store error"} {
			t.Run(route+"/"+outcome, func(t *testing.T) {
				_, base, _, raw, secret := setup(t)
				now := base.now
				token, digest, err := credential.Issue(strings.NewReader(strings.Repeat("x", 32)))
				if err != nil {
					t.Fatal(err)
				}
				base.tokens = append(base.tokens, store.PersonalToken{ID: 1, AccountID: 1, TokenHash: digest, ExpiresAt: now.Add(time.Hour)})
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				db := &uncertainStore{fakeStore: base, fail: outcome == "store error"}
				if outcome == "canceled" {
					db.cancel = cancel
				}
				manager := &websession.Manager{Store: base, Clock: func() time.Time { return now }, PublicURL: "https://example.invalid"}
				verifier := &credential.Verifier{Store: base, Clock: func() time.Time { return now }, Audience: "mcp"}
				handler := manager.Middleware(manager.CSRF(New(Config{Store: db, Sessions: manager, Verifier: verifier, Limiter: limits.New(func() time.Time { return now })})))
				expect(t, call(handler, raw, secret, "GET", "/api/v1/me", ""), 200)
				if _, err := verifier.Verify(context.Background(), token); err != nil {
					t.Fatal(err)
				}
				path := "/api/v1/me/tokens/1"
				if route == "deletion" {
					path = "/api/v1/me"
				}
				req := httptest.NewRequest(http.MethodDelete, "https://example.invalid"+path, nil).WithContext(ctx)
				req.AddCookie(&http.Cookie{Name: "__Host-session", Value: raw})
				req.Header.Set("Origin", "https://example.invalid")
				req.Header.Set("X-CSRF-Token", secret)
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, req)
				expect(t, w, 503)
				if _, err := verifier.Verify(context.Background(), token); !errors.Is(err, credential.ErrUnauthorized) {
					t.Error("revoked token remains cached")
				}
				if route == "deletion" {
					w = call(handler, raw, secret, "GET", "/api/v1/me", "")
					if w.Code != 401 {
						t.Errorf("revoked session remains cached: status %d", w.Code)
					}
				}
			})
		}
	}
}
