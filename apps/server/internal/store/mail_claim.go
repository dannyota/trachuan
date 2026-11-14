package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func (q *Queries) ClaimDueMail(ctx context.Context, at time.Time) (MailOutbox, error) {
	db, ok := q.db.(TransactionBeginner)
	if !ok {
		return MailOutbox{}, fmt.Errorf("mail store cannot begin transactions")
	}
	tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return MailOutbox{}, fmt.Errorf("begin mail claim: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(739215,2)`); err != nil {
		return MailOutbox{}, fmt.Errorf("lock mail budget: %w", err)
	}
	if _, err = tx.Exec(ctx, `DELETE FROM mail_delivery WHERE COALESCE(sent_at,reserved_at)<LEAST(date_trunc('month',$1::timestamptz AT TIME ZONE 'UTC') AT TIME ZONE 'UTC',$1::timestamptz-interval '1 hour')`, at); err != nil {
		return MailOutbox{}, fmt.Errorf("purge mail accounting: %w", err)
	}
	row, err := New(tx).ClaimMailCandidate(ctx, at)
	if err != nil {
		return row, err
	}
	if row.ExpiresAt.After(at) && row.Attempts <= 6 {
		_, err = tx.Exec(ctx, `INSERT INTO mail_delivery(outbox_id,attempt,class,reserved_at) VALUES ($1,$2,$3,$4)`, row.ID, row.Attempts, row.Class, at)
		if err != nil {
			return row, fmt.Errorf("reserve mail budget: %w", err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return row, fmt.Errorf("commit mail claim: %w", err)
	}
	return row, nil
}
