// Package admin provides account and personal token administration.
package admin

import (
	"context"
	"fmt"
	"io"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
)

// Store supplies account administration and personal token writes.
type Store interface {
	CreateAccount(context.Context, store.CreateAccountParams) (store.Account, error)
	GetAccount(context.Context, int64) (store.Account, error)
	BlockAccount(context.Context, int64) (store.BlockAccountRow, error)
	CreatePersonalToken(context.Context, store.CreatePersonalTokenParams) (store.PersonalToken, error)
	RevokePersonalToken(context.Context, string) error
}

// Service administers accounts and tokens. Clock and Rand default to the system
// clock and crypto/rand.Reader. Configure fields before use.
type Service struct {
	Store Store
	Clock func() time.Time
	Rand  io.Reader
}

type failure string

func (e failure) Error() string { return string(e) }

const errStore failure = "account store unavailable"

// CreateAccount creates the identity supplied by the administrator.
func (s *Service) CreateAccount(ctx context.Context, googleSub, email string) (store.Account, error) {
	account, err := s.Store.CreateAccount(ctx, store.CreateAccountParams{GoogleSub: googleSub, Email: email})
	if err != nil {
		// Store errors may include identity or credential parameters.
		return store.Account{}, fmt.Errorf("create account: %w", errStore)
	}
	return account, nil
}

// BlockAccount blocks an account, advances its epoch and revokes its tokens in
// one store operation. Other processes observe this within their cache window.
func (s *Service) BlockAccount(ctx context.Context, accountID int64) error {
	if _, err := s.Store.BlockAccount(ctx, accountID); err != nil {
		return fmt.Errorf("block account: %w", errStore)
	}
	return nil
}
