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

// credentialStore serializes creation with account block and deletion. The lock
// is a separate statement so READ COMMITTED takes a fresh snapshot after waiting.
type credentialStore struct {
	*store.Queries
	db transactionBeginner
}

func (s credentialStore) withAccount(ctx context.Context, id int64, epoch int32, write func(*store.Queries) error) error {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin credential write: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.WithTx(tx)
	account, err := q.LockCredentialAccount(ctx, id)
	if err != nil {
		return fmt.Errorf("lock credential account: %w", err)
	}
	if account.BlockedAt != nil || account.DeleteRequestedAt != nil || account.AuthEpoch != epoch {
		return credential.ErrUnauthorized
	}
	if err := write(q); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit credential write: %w", err)
	}
	return nil
}

func (s credentialStore) CreatePersonalToken(ctx context.Context, p store.CreatePersonalTokenParams) (store.PersonalToken, error) {
	var row store.PersonalToken
	err := s.withAccount(ctx, p.AccountID, p.AuthEpoch, func(q *store.Queries) error {
		var err error
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
		var err error
		row, err = q.CreateSession(ctx, p)
		return err
	})
	return row, err
}
