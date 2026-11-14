package accountapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
	"github.com/jackc/pgx/v5"
)

type securityStore struct {
	*fakeStore
	passwordID int64
	totpID     int64
	err        error
	totpErr    error
	queried    []int64
}

func (s *securityStore) GetPasswordCredential(_ context.Context, id int64) (store.PasswordCredential, error) {
	s.queried = append(s.queried, id)
	if s.err != nil {
		return store.PasswordCredential{}, s.err
	}
	if id != s.passwordID {
		return store.PasswordCredential{}, pgx.ErrNoRows
	}
	return store.PasswordCredential{AccountID: id}, nil
}
func (s *securityStore) HasTOTP(_ context.Context, id int64) (bool, error) {
	s.queried = append(s.queried, id)
	return id == s.totpID, s.totpErr
}
func TestMe_SecurityBelongsToCaller(t *testing.T) {
	for _, enrolled := range []bool{false, true} {
		for _, own := range []bool{false, true} {
			_, base, verifier, raw, csrf := setup(t)
			id := int64(2)
			if own {
				id = 1
				sub := "synthetic-sub"
				base.account.GoogleSub = &sub
			}
			db := &securityStore{fakeStore: base, passwordID: id, totpID: id}
			now := func() time.Time { return base.now }
			sessions := &websession.Manager{Store: base, Clock: now, PublicURL: "https://example.invalid"}
			handler := sessions.Middleware(sessions.CSRF(New(Config{Store: db, Sessions: sessions, Verifier: verifier, Limiter: limits.New(now), TOTPEnrollment: enrolled})))
			response := call(handler, raw, csrf, http.MethodGet, "/api/v1/me?account_id=2", "")
			expect(t, response, 200)
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			for key, want := range map[string]bool{"password": own, "google": own, "totp": own, "totp_enrollment": enrolled} {
				if body[key] != want {
					t.Errorf("%s=%v, want %v", key, body[key], want)
				}
			}
			for _, queried := range db.queried {
				if queried != 1 {
					t.Fatal("queried another account")
				}
			}
		}
	}
}

func TestMe_SecurityReadFailure(t *testing.T) {
	for _, failed := range []string{"password", "totp"} {
		t.Run(failed, func(t *testing.T) {
			_, base, verifier, raw, csrf := setup(t)
			db := &securityStore{fakeStore: base}
			failure := errors.New("synthetic read failure")
			if failed == "password" {
				db.err = failure
			} else {
				db.totpErr = failure
			}
			now := func() time.Time { return base.now }
			sessions := &websession.Manager{Store: base, Clock: now, PublicURL: "https://example.invalid"}
			handler := sessions.Middleware(sessions.CSRF(New(Config{Store: db, Sessions: sessions, Verifier: verifier, Limiter: limits.New(now)})))
			response := call(handler, raw, csrf, http.MethodGet, "/api/v1/me", "")
			expect(t, response, 503)
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if _, ok := body["csrf_token"]; ok {
				t.Fatal("partial account response")
			}
		})
	}
}
