//go:build integration

package app

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/weblogin"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLoginStore_PersistsAndClaims(t *testing.T) {
	ctx := context.Background()
	databaseURL := testpg.NewDatabase(t)
	if err := migrate.Up(ctx, databaseURL); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	s := loginStore{queries: store.New(pool)}
	tx := weblogin.LoginTransaction{HandleHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", State: "state", Nonce: "nonce", PKCEVerifier: "verifier", NextPath: "/next", RedirectURI: "http://localhost/callback", ExpiresAt: time.Now().Add(time.Hour)}
	if err = s.CreateLoginTransaction(ctx, tx); err != nil {
		t.Fatal(err)
	}
	got, err := s.ClaimLoginTransaction(ctx, weblogin.ClaimLoginTransactionParams{HandleHash: tx.HandleHash, Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if got.HandleHash != tx.HandleHash || got.State != tx.State || got.Nonce != tx.Nonce || got.PKCEVerifier != tx.PKCEVerifier || got.NextPath != tx.NextPath || got.RedirectURI != tx.RedirectURI {
		t.Fatal("transaction fields did not round trip")
	}
	if _, err = s.ClaimLoginTransaction(ctx, weblogin.ClaimLoginTransactionParams{HandleHash: tx.HandleHash, Now: time.Now()}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("second claim: %v", err)
	}
}
