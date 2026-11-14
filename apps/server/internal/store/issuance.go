package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type TransactionBeginner interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}

func WithAccount(ctx context.Context, db TransactionBeginner, id int64, epoch int32, write func(*Queries) error) error {
	tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin credential write: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	account, err := LockAccount(ctx, tx, id)
	if err != nil {
		return err
	}
	if account.BlockedAt != nil || account.DeleteRequestedAt != nil || account.AuthEpoch != epoch {
		return ErrUnauthorized
	}
	if err := write(New(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit credential write: %w", err)
	}
	return nil
}
