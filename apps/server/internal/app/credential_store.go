package app

import (
	"context"
	"errors"
	"fmt"

	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

type transactionBeginner interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}

type credentialStore struct {
	*store.Queries
	db transactionBeginner
}

func (s credentialStore) withAccount(ctx context.Context, id int64, epoch int32, write func(*store.Queries) error) error {
	return store.WithAccount(ctx, s.db, id, epoch, write)
}

func (s credentialStore) CreatePersonalToken(ctx context.Context, p store.CreatePersonalTokenParams) (store.PersonalToken, error) {
	var row store.PersonalToken
	err := s.withAccount(ctx, p.AccountID, p.AuthEpoch, func(q *store.Queries) error {
		enrolled, err := q.HasTOTP(ctx, p.AccountID)
		if err != nil {
			return fmt.Errorf("read token factor: %w", err)
		}
		if enrolled {
			return store.ErrFactorEnrolled
		}
		row, err = q.CreatePersonalToken(ctx, p)
		if errors.Is(err, pgx.ErrNoRows) {
			return credential.ErrTokenLimit
		}
		return err
	})
	return row, err
}

func (s credentialStore) CreateSession(ctx context.Context, p store.CreateSessionParams) (store.CreateSessionRow, error) {
	var row store.CreateSessionRow
	err := s.withAccount(ctx, p.AccountID, p.AuthEpoch, func(q *store.Queries) error {
		enrolled, err := q.HasTOTP(ctx, p.AccountID)
		if err != nil {
			return fmt.Errorf("read session factor: %w", err)
		}
		if enrolled {
			return store.ErrLoginFailed
		}
		row, err = q.CreateSession(ctx, p)
		return err
	})
	return row, err
}
