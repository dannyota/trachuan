package passwordauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/accountflow"
	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
)

type changedAuthority struct {
	sessionStore
	now   time.Time
	epoch int32
}

func (s *changedAuthority) GetSession(ctx context.Context, hash string) (store.GetSessionRow, error) {
	row, err := s.sessionStore.GetSession(ctx, hash)
	row.AccountAuthEpoch = s.epoch
	return row, err
}
func (s *changedAuthority) GetCredential(context.Context, string) (store.GetCredentialRow, error) {
	return store.GetCredentialRow{AccountID: 1, AccountAuthEpoch: s.epoch, ExpiresAt: s.now.Add(time.Hour)}, nil
}
func (s *changedAuthority) GetToken(context.Context, string) (store.GetTokenRow, error) {
	return store.GetTokenRow{Kind: "access", GrantID: 1, AccountID: 1, Resource: "https://example.test/mcp", Scope: "mcp:read", AccountAuthEpoch: s.epoch, ExpiresAt: s.now.Add(time.Hour), FamilyExpiresAt: s.now.Add(time.Hour)}, nil
}
func (*changedAuthority) TouchPersonalToken(context.Context, string) error { return nil }
func (*changedAuthority) TouchOAuthGrant(context.Context, int64) error     { return nil }

type uncertainFactor struct {
	factorFlow
	authority *changedAuthority
}

func (s uncertainFactor) changed() (store.CreateSessionRow, error) {
	s.authority.epoch++
	return store.CreateSessionRow{AccountID: 1}, errors.New("synthetic commit acknowledgement lost")
}
func (s uncertainFactor) ConfirmTOTP(context.Context, accountflow.FactorChange) (store.CreateSessionRow, []string, error) {
	row, err := s.changed()
	return row, nil, err
}
func (s uncertainFactor) RemoveTOTP(context.Context, accountflow.FactorChange) (store.CreateSessionRow, error) {
	return s.changed()
}
func (s uncertainFactor) ReplaceRecoveryCodes(context.Context, accountflow.FactorChange) (store.CreateSessionRow, []string, error) {
	row, err := s.changed()
	return row, nil, err
}

func TestFactor_UncertainCommitEvictsCachedAuthority(t *testing.T) {
	for _, kind := range []string{"enrollment", "removal", "recovery replacement"} {
		t.Run(kind, func(t *testing.T) {
			db := &changedAuthority{now: time.Now()}
			sessions := &websession.Manager{Store: db, PublicURL: "https://example.test"}
			bearer := &credential.Verifier{Store: db, Audience: sessions.PublicURL + "/mcp", Clock: func() time.Time { return db.now }}
			tokens := []string{"trc_" + strings.Repeat("A", 43), "tra_" + strings.Repeat("B", 43)}
			for _, token := range tokens {
				if _, err := bearer.Verify(t.Context(), token); err != nil {
					t.Fatal(err)
				}
			}
			h := Handler{Factors: uncertainFactor{authority: db}, Sessions: sessions, Enrollment: true, Evict: func(id int64) { sessions.EvictAccount(id); bearer.EvictAccount(id) }}
			method, path := http.MethodPost, "/api/v1/me/totp/confirm"
			handler := http.HandlerFunc(h.TOTPConfirm)
			switch kind {
			case "removal":
				method, path, handler = http.MethodDelete, "/api/v1/me/totp", h.TOTPRemove
			case "recovery replacement":
				path, handler = "/api/v1/me/recovery-codes", h.RecoveryCodes
			}
			r := httptest.NewRequest(method, path, strings.NewReader(`{"code":"123456"}`))
			r.AddCookie(&http.Cookie{Name: "__Host-session", Value: strings.Repeat("A", 43)})
			r.Header.Set("Origin", sessions.PublicURL)
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-CSRF-Token", strings.Repeat("A", 43))
			w := httptest.NewRecorder()
			sessions.Middleware(sessions.CSRF(handler)).ServeHTTP(w, r)
			if w.Code != 503 || len(w.Result().Cookies()) != 0 {
				t.Fatal("uncertain commit issued session", w.Code)
			}
			for _, token := range tokens {
				if _, err := bearer.Verify(t.Context(), token); !errors.Is(err, credential.ErrUnauthorized) {
					t.Error("cached bearer survived uncertain commit", err)
				}
			}
			probe := httptest.NewRecorder()
			sessions.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })).ServeHTTP(probe, r)
			if probe.Code != 401 {
				t.Fatal("cached session survived uncertain commit")
			}
		})
	}
}
