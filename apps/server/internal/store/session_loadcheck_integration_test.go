//go:build integration

package store_test

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
)

func TestTouchSession_LoadcheckExpiryStaysFixed(t *testing.T) {
	ctx, conn, q := setup(t)
	for _, tc := range []struct {
		name, sub string
		fixed     bool
	}{
		{name: "normal", sub: "sliding-subject"},
		{name: "loadcheck", sub: "loadcheck:fixed", fixed: true},
		{name: "similar prefix", sub: "loadcheck-sliding"},
		{name: "password", fixed: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var id int64
			if tc.sub == "" {
				tx, err := conn.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = tx.Rollback(ctx) }()
				if err = tx.QueryRow(ctx, `INSERT INTO account(email) VALUES ('password-slide@example.test') RETURNING id`).Scan(&id); err != nil {
					t.Fatal(err)
				}
				if _, err = tx.Exec(ctx, `INSERT INTO password_credential(account_id,hash,version) VALUES ($1,'synthetic',1)`, id); err != nil {
					t.Fatal(err)
				}
				if err = tx.Commit(ctx); err != nil {
					t.Fatal(err)
				}
			} else {
				account, err := q.CreateAccount(ctx, store.CreateAccountParams{GoogleSub: tc.sub, Email: strings.ReplaceAll(tc.name, " ", "-") + "@example.test"})
				if err != nil {
					t.Fatal(err)
				}
				id = account.ID
			}
			digest := sha256.Sum256([]byte(tc.name))
			hash := hex.EncodeToString(digest[:])
			sessionID := webSession(t, ctx, conn, id, hash)
			if _, err := conn.Exec(ctx, `UPDATE web_session SET expires_at=now()+interval '2 hours',last_seen_at=now()-interval '2 hours' WHERE id=$1`, sessionID); err != nil {
				t.Fatal(err)
			}
			before, err := q.GetSession(ctx, hash)
			if err != nil {
				t.Fatal(err)
			}
			if count, touchErr := q.TouchSession(ctx, hash); touchErr != nil || count != 1 {
				t.Fatalf("touch count=%d error=%v", count, touchErr)
			}
			after, err := q.GetSession(ctx, hash)
			if err != nil || !after.LastSeenAt.After(before.LastSeenAt) {
				t.Fatal("session activity was not recorded")
			}
			if tc.fixed {
				if !after.ExpiresAt.Equal(before.ExpiresAt) {
					t.Fatal("loadcheck expiry moved")
				}
			} else if after.ExpiresAt.Before(time.Now().Add(30*24*time.Hour-time.Minute)) || !after.ExpiresAt.After(before.ExpiresAt) {
				t.Fatal("normal session did not slide to 30 days")
			}
			if _, ageErr := conn.Exec(ctx, `UPDATE web_session SET created_at=now()-interval '89 days',last_seen_at=now()-interval '2 hours' WHERE id=$1`, sessionID); ageErr != nil {
				t.Fatal(ageErr)
			}
			if count, touchErr := q.TouchSession(ctx, hash); touchErr != nil || count != 1 {
				t.Fatalf("second touch count=%d error=%v", count, touchErr)
			}
			after, err = q.GetSession(ctx, hash)
			if err != nil {
				t.Fatal(err)
			}
			if tc.fixed {
				if !after.ExpiresAt.Equal(before.ExpiresAt) {
					t.Fatal("second touch extended loadcheck expiry")
				}
			} else if !after.ExpiresAt.Equal(after.CreatedAt.Add(90 * 24 * time.Hour)) {
				t.Fatal("normal session exceeded its absolute cap")
			}
		})
	}
}
