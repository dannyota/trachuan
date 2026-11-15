package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (q *Queries) RestoreFence(ctx context.Context) error {
	if err := q.requireCurrentAuthSchema(ctx); err != nil {
		return err
	}
	db, ok := q.db.(TransactionBeginner)
	if !ok {
		return fmt.Errorf("restore fence store cannot begin transactions")
	}
	tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin restore fence: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `LOCK TABLE account, framework_entitlement IN ACCESS EXCLUSIVE MODE`); err != nil {
		return fmt.Errorf("lock restore fence: %w", err)
	}
	if err = New(tx).ApplyRestoreFence(ctx); err != nil {
		return fmt.Errorf("apply restore fence: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit restore fence: %w", err)
	}
	return nil
}
