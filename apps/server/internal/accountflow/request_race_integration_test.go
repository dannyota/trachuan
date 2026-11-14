//go:build integration

package accountflow

import (
	"context"
	"testing"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

type pausedLookupDB struct {
	store.TransactionBeginner
	read, resume chan struct{}
}
type pausedLookupTx struct {
	pgx.Tx
	read, resume chan struct{}
}
type pausedLookupRow struct {
	pgx.Row
	ctx          context.Context
	read, resume chan struct{}
}

func (db pausedLookupDB) BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error) {
	tx, err := db.TransactionBeginner.BeginTx(ctx, opts)
	return pausedLookupTx{tx, db.read, db.resume}, err
}
func (tx pausedLookupTx) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	row := tx.Tx.QueryRow(ctx, query, args...)
	if query == `SELECT id FROM account WHERE email=$1` {
		return pausedLookupRow{row, ctx, tx.read, tx.resume}
	}
	return row
}
func (r pausedLookupRow) Scan(dest ...any) error {
	err := r.Row.Scan(dest...)
	close(r.read)
	select {
	case <-r.resume:
		return err
	case <-r.ctx.Done():
		return r.ctx.Err()
	}
}
func TestResetRequest_ConcurrentRegistrationNeedsLockedOwner(t *testing.T) {
	s, db := fixture(t)
	read, resume := make(chan struct{}), make(chan struct{})
	s.DB = pausedLookupDB{db, read, resume}
	result := make(chan error, 1)
	go func() { result <- s.RequestMail(t.Context(), "reset", "person@example.test") }()
	<-read
	a, err := store.New(db).CreateAccount(t.Context(), store.CreateAccountParams{GoogleSub: "synthetic", Email: "person@example.test"})
	if err == nil {
		_, err = db.Exec(t.Context(), `INSERT INTO password_credential(account_id,hash) VALUES ($1,'old')`, a.ID)
	}
	close(resume)
	if err != nil {
		t.Fatal(err)
	}
	if err = <-result; err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow(t.Context(), `SELECT count(*) FROM email_link`).Scan(&count); err != nil || count != 0 {
		t.Fatal("issued for an unlocked owner", err)
	}
}
