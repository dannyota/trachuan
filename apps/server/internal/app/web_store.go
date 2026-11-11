package app

import (
	"context"
	"fmt"

	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/weblogin"
)

type loginStore struct{ queries *store.Queries }

func (s loginStore) CreateLoginTransaction(ctx context.Context, tx weblogin.LoginTransaction) error {
	_, err := s.queries.CreateLoginTransaction(ctx, store.CreateLoginTransactionParams{HandleHash: tx.HandleHash, State: tx.State, Nonce: tx.Nonce, PkceVerifier: tx.PKCEVerifier, NextPath: tx.NextPath, RedirectUri: tx.RedirectURI, ExpiresAt: tx.ExpiresAt})
	if err != nil {
		return fmt.Errorf("create login transaction: %w", err)
	}
	return nil
}
func (s loginStore) ClaimLoginTransaction(ctx context.Context, p weblogin.ClaimLoginTransactionParams) (weblogin.LoginTransaction, error) {
	row, err := s.queries.ClaimLoginTransaction(ctx, p.HandleHash)
	if err != nil {
		return weblogin.LoginTransaction{}, fmt.Errorf("claim login transaction: %w", err)
	}
	return weblogin.LoginTransaction{HandleHash: row.HandleHash, State: row.State, Nonce: row.Nonce, PKCEVerifier: row.PkceVerifier, NextPath: row.NextPath, RedirectURI: row.RedirectUri, CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt}, nil
}
func (s loginStore) UpsertGoogleAccount(ctx context.Context, p weblogin.UpsertGoogleAccountParams) (weblogin.GoogleAccount, error) {
	row, err := s.queries.UpsertGoogleAccount(ctx, store.UpsertGoogleAccountParams{GoogleSub: p.Sub, Email: p.Email})
	if err != nil {
		return weblogin.GoogleAccount{}, fmt.Errorf("upsert Google account: %w", err)
	}
	return weblogin.GoogleAccount{ID: row.ID, Blocked: row.BlockedAt != nil, DeleteRequested: row.DeleteRequestedAt != nil}, nil
}
