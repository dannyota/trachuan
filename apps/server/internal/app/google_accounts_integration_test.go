//go:build integration

package app

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
)

func TestGoogle_SubjectOnlyAndFixedEmail(t *testing.T) {
	h, pool, p := signupConfigHandler(t, nil)
	w := httptest.NewRecorder()
	h.Callback.ServeHTTP(w, signupRequest(t, h, p, "first", "first-sub", "Owner@EXAMPLE.TEST"))
	assertSignupResponse(t, w, "/destination", true)
	w = httptest.NewRecorder()
	h.Callback.ServeHTTP(w, signupRequest(t, h, p, "return", "first-sub", "changed@example.test"))
	assertSignupResponse(t, w, "/destination", true)
	var email string
	var proof bool
	if err := pool.QueryRow(t.Context(), `SELECT email FROM account WHERE google_sub='first-sub'`).Scan(&email); err != nil || email != "Owner@example.test" {
		t.Fatal("returning sign-in changed email")
	}
	if err := pool.QueryRow(t.Context(), `SELECT bool_and(proof_at IS NOT NULL AND factor_at IS NULL) FROM web_session`).Scan(&proof); err != nil || !proof {
		t.Fatal("sign-in proof missing")
	}
	w = httptest.NewRecorder()
	h.Callback.ServeHTTP(w, signupRequest(t, h, p, "taken", "new-sub", "Owner@example.test"))
	assertSignupResponse(t, w, "/login?error=login_failed", false)
	assertSignupRows(t, pool, 1, 2, 3)
}

func TestGoogle_RejectsInvalidEmailAndStagesEnrolledAccount(t *testing.T) {
	for _, enrolled := range []bool{false, true} {
		t.Run(map[bool]string{false: "invalid", true: "enrolled"}[enrolled], func(t *testing.T) {
			h, pool, p := signupConfigHandler(t, nil)
			email := "\"owner\"@example.test"
			if enrolled {
				email = "owner@example.test"
				if _, err := pool.Exec(t.Context(), `INSERT INTO account(google_sub,email) VALUES ('enrolled','owner@example.test');
 INSERT INTO totp_credential(account_id,sealed_secret,key_id) SELECT id,'synthetic','synthetic' FROM account;`); err != nil {
					t.Fatal(err)
				}
			}
			w := httptest.NewRecorder()
			h.Callback.ServeHTTP(w, signupRequest(t, h, p, "refused", "enrolled", email))
			destination := "/login?error=login_failed"
			if enrolled {
				destination = "/login/factor"
			}
			assertSignupResponse(t, w, destination, false)
			var pendingCount int
			if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM pending_auth WHERE next='/destination' AND primary_kind='google'`).Scan(&pendingCount); err != nil {
				t.Fatal(err)
			}
			if enrolled {
				found := false
				for _, cookie := range w.Result().Cookies() {
					if cookie.Name == "__Host-pending" && cookie.MaxAge == 300 {
						found = true
					}
				}
				if pendingCount != 1 || !found {
					t.Fatal("Google sign-in did not stage factor")
				}
			} else if pendingCount != 0 {
				t.Fatal("invalid email staged pending authentication")
			}
			count := 0
			if enrolled {
				count = 1
			}
			assertSignupRows(t, pool, count, 0, 1)
		})
	}
}

func TestGoogle_SessionFailureRollsBackNewAccount(t *testing.T) {
	_, pool, _ := signupConfigHandler(t, nil)
	q := store.New(pool)
	first := store.GoogleSessionParams{Identity: store.GoogleIdentity{Sub: "first", Email: "first@example.test"}, Session: store.CreateSessionParams{TokenHash: strings.Repeat("a", 64), CsrfSecret: "synthetic", ExpiresAt: time.Now().Add(time.Hour)}}
	if _, err := q.CreateGoogleSession(t.Context(), first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.Identity = store.GoogleIdentity{Sub: "second", Email: "second@example.test"}
	if _, err := q.CreateGoogleSession(t.Context(), second); err == nil {
		t.Fatal("duplicate session succeeded")
	}
	var accounts, sessions int
	if err := pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM account),(SELECT count(*) FROM web_session)`).Scan(&accounts, &sessions); err != nil || accounts != 1 || sessions != 1 {
		t.Fatal("failed session left account or session")
	}
}
