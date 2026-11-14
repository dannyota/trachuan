package admin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"danny.vn/trachuan/apps/server/internal/accountemail"
	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5/pgconn"
)

type Store interface {
	CreateAccount(context.Context, store.CreateAccountParams) (store.Account, error)
	GetAccount(context.Context, int64) (store.Account, error)
	BlockAccount(context.Context, int64) (store.BlockAccountRow, error)
	CreatePersonalToken(context.Context, store.CreatePersonalTokenParams) (store.PersonalToken, error)
	RevokePersonalToken(context.Context, string) error
}

type Service struct {
	Store Store
	Clock func() time.Time
	Rand  io.Reader
}

type failure string

func (e failure) Error() string { return string(e) }

const (
	errStore      failure = "account store unavailable"
	errEmailTaken failure = "email already belongs to an account"
)

func (s *Service) CreateAccount(ctx context.Context, googleSub, email string) (store.Account, error) {
	email, err := accountemail.Parse(email)
	if err != nil {
		return store.Account{}, fmt.Errorf("create account: invalid email address")
	}
	account, err := s.Store.CreateAccount(ctx, store.CreateAccountParams{GoogleSub: googleSub, Email: email})
	if err != nil {
		var conflict *pgconn.PgError
		if errors.As(err, &conflict) && conflict.Code == "23505" && conflict.ConstraintName == "account_email_key" {
			return store.Account{}, fmt.Errorf("create account: %w", errEmailTaken)
		}
		return store.Account{}, fmt.Errorf("create account: %w", errStore)
	}
	return account, nil
}

func (s *Service) BlockAccount(ctx context.Context, accountID int64) error {
	if _, err := s.Store.BlockAccount(ctx, accountID); err != nil {
		return fmt.Errorf("block account: %w", errStore)
	}
	return nil
}
