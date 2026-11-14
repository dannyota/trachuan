package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type BlockAccountRow = Account
type RequestDeletionRow = Account

func LockAccount(ctx context.Context, tx pgx.Tx, id int64) (Account, error) {
	account, err := New(tx).LockCredentialAccount(ctx, id)
	if err != nil {
		return Account{}, fmt.Errorf("lock account: %w", err)
	}
	return account, nil
}

func BumpEpoch(ctx context.Context, tx pgx.Tx, id int64) (int64, error) {
	if _, err := LockAccount(ctx, tx, id); err != nil {
		return 0, err
	}
	row, err := New(tx).BumpAccountEpoch(ctx, id)
	if err != nil {
		return 0, fmt.Errorf("bump account epoch: %w", err)
	}
	return row.ID, nil
}

func (q *Queries) BlockAccount(ctx context.Context, id int64) (BlockAccountRow, error) {
	return q.endAccount(ctx, id, false)
}
func (q *Queries) RequestDeletion(ctx context.Context, id int64) (RequestDeletionRow, error) {
	return q.endAccount(ctx, id, true)
}
func (q *Queries) endAccount(ctx context.Context, id int64, deleting bool) (Account, error) {
	db, ok := q.db.(TransactionBeginner)
	if !ok {
		return Account{}, fmt.Errorf("account store cannot begin transactions")
	}
	tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return Account{}, fmt.Errorf("begin account change: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = BumpEpoch(ctx, tx, id); err != nil {
		return Account{}, err
	}
	var account Account
	if deleting {
		account, err = New(tx).MarkAccountDeleting(ctx, id)
	} else {
		account, err = New(tx).MarkAccountBlocked(ctx, id)
	}
	if err != nil {
		return Account{}, fmt.Errorf("mark account state: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Account{}, fmt.Errorf("commit account change: %w", err)
	}
	return account, nil
}
