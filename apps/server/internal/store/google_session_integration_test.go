//go:build integration

package store_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
)

func TestGoogleSession_ReservedSubjectsCannotCreateOrEnterAccounts(t *testing.T) {
	for _, name := range []string{"new", "existing"} {
		t.Run(name, func(t *testing.T) {
			ctx, conn, q := setup(t)
			subject := "loadcheck:" + name
			wantAccounts := 0
			var id int64
			if name == "existing" {
				account, err := q.CreateAccount(ctx, store.CreateAccountParams{GoogleSub: subject, Email: "reserved@example.test"})
				if err != nil {
					t.Fatal(err)
				}
				id, wantAccounts = account.ID, 1
			}
			row, err := q.CreateGoogleSession(ctx, store.GoogleSessionParams{Identity: store.GoogleIdentity{Sub: subject, Email: "reserved@example.test"}, Session: store.CreateSessionParams{TokenHash: strings.Repeat("a", 64), CsrfSecret: "synthetic", ExpiresAt: time.Now().Add(time.Hour)}})
			if !errors.Is(err, store.ErrLoginFailed) || row.ID != 0 {
				t.Fatal("reserved subject was not rejected")
			}
			var accounts, sessions int
			if countErr := conn.QueryRow(ctx, `SELECT (SELECT count(*) FROM account),(SELECT count(*) FROM web_session)`).Scan(&accounts, &sessions); countErr != nil || accounts != wantAccounts || sessions != 0 {
				t.Fatal("rejected subject wrote an account or session")
			}
			if id != 0 {
				account, lookupErr := q.GetAccount(ctx, id)
				if lookupErr != nil || account.LastLoginAt != nil {
					t.Fatal("rejected subject changed login history")
				}
			}
		})
	}
}
