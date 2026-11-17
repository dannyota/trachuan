//go:build integration

package app

import (
	"net/http/httptest"
	"testing"

	"danny.vn/trachuan/apps/server/internal/store"
)

func TestGoogle_ReservedSubjectsRefusedForNewAndExistingAccounts(t *testing.T) {
	for _, name := range []string{"new", "existing"} {
		t.Run(name, func(t *testing.T) {
			h, pool, provider := signupConfigHandler(t, nil)
			subject := "loadcheck:" + name
			accounts := 0
			if name == "existing" {
				if _, err := store.New(pool).CreateAccount(t.Context(), store.CreateAccountParams{GoogleSub: subject, Email: "reserved@example.test"}); err != nil {
					t.Fatal(err)
				}
				accounts = 1
			}
			response := httptest.NewRecorder()
			h.Callback.ServeHTTP(response, signupRequest(t, h, provider, "reserved-code", subject, "reserved@example.test"))
			assertSignupResponse(t, response, "/login?error=login_failed", false)
			assertSignupRows(t, pool, accounts, 0, 1)
		})
	}
}
