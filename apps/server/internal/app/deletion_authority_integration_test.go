//go:build integration

package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
)

func TestDeletion_RevocationWinsAccountLockRace(t *testing.T) {
	for _, change := range []string{"session revocation", "epoch bump"} {
		t.Run(change, func(t *testing.T) {
			ctx, pool, s, id := boundsFixture(t)
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			session, err := s.CreateSession(ctx, store.CreateSessionParams{AccountID: id, TokenHash: strings.Repeat("a", 64), CsrfSecret: "synthetic", ExpiresAt: time.Now().Add(time.Hour)})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.CreatePersonalToken(ctx, store.CreatePersonalTokenParams{AccountID: id, TokenHash: strings.Repeat("b", 64), Name: "synthetic", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
				t.Fatal(err)
			}
			mutation, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = mutation.Rollback(ctx) }()
			if _, err = store.LockAccount(ctx, mutation, id); err != nil {
				t.Fatal(err)
			}
			var expectedEpoch int32
			expectedTokens := 1
			if change == "session revocation" {
				if _, err = store.New(mutation).RevokeSession(ctx, session.TokenHash); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err = store.BumpEpoch(ctx, mutation, id); err != nil {
					t.Fatal(err)
				}
				expectedEpoch = 1
				expectedTokens = 0
			}
			observed := &issuerPool{Pool: pool, lock: make(chan struct{})}
			result := make(chan error, 1)
			go func() {
				_, deleteErr := store.New(observed).RequestDeletion(ctx, store.RequestDeletionParams{AccountID: id, SessionID: session.ID, AuthEpoch: session.AuthEpoch})
				result <- deleteErr
			}()
			select {
			case <-observed.lock:
			case <-ctx.Done():
				t.Fatal("deletion did not take the account lock")
			}
			if err = mutation.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case deleteErr := <-result:
				if !errors.Is(deleteErr, store.ErrUnauthorized) {
					t.Fatalf("stale deletion error=%v", deleteErr)
				}
			case <-ctx.Done():
				t.Fatal("deletion did not finish")
			}
			account, err := s.GetAccount(ctx, id)
			if err != nil || account.AuthEpoch != expectedEpoch || account.BlockedAt != nil || account.DeleteRequestedAt != nil {
				t.Fatal("refused deletion changed account state")
			}
			var liveTokens int
			if err = pool.QueryRow(ctx, `SELECT count(*) FROM personal_token WHERE account_id=$1 AND revoked_at IS NULL`, id).Scan(&liveTokens); err != nil || liveTokens != expectedTokens {
				t.Fatal("refused deletion revoked other credentials")
			}
		})
	}
}
