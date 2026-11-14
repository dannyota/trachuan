package store

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"danny.vn/trachuan/apps/server/internal/accountemail"
	"github.com/jackc/pgx/v5"
)

func (q *Queries) CreateGoogleSession(ctx context.Context, p GoogleSessionParams) (CreateSessionRow, error) {
	db, ok := q.db.(TransactionBeginner)
	if !ok {
		return CreateSessionRow{}, fmt.Errorf("login store cannot begin transactions")
	}
	tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return CreateSessionRow{}, fmt.Errorf("begin Google login: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	current := New(tx)
	account, err := current.FindGoogleAccount(ctx, p.Identity.Sub)
	if errors.Is(err, pgx.ErrNoRows) {
		email, parseErr := accountemail.Parse(p.Identity.Email)
		if parseErr != nil {
			return CreateSessionRow{}, ErrLoginFailed
		}
		taken, lookupErr := current.AccountEmailExists(ctx, email)
		if lookupErr != nil {
			return CreateSessionRow{}, fmt.Errorf("check signup address: %w", lookupErr)
		}
		if !taken {
			if p.Identity.SignupEmails != nil && !slices.Contains(p.Identity.SignupEmails, email) {
				return CreateSessionRow{}, ErrSignupClosed
			}
			account, err = current.InsertGoogleAccount(ctx, InsertGoogleAccountParams{GoogleSub: p.Identity.Sub, Email: email})
		}
		if taken || errors.Is(err, pgx.ErrNoRows) {
			account, err = current.FindGoogleAccount(ctx, p.Identity.Sub)
			if errors.Is(err, pgx.ErrNoRows) {
				return CreateSessionRow{}, ErrLoginFailed
			}
		}
	}
	if err != nil {
		return CreateSessionRow{}, fmt.Errorf("find Google account: %w", err)
	}
	account, err = LockAccount(ctx, tx, account.ID)
	if err != nil {
		return CreateSessionRow{}, err
	}
	if account.GoogleSub == nil || *account.GoogleSub != p.Identity.Sub {
		return CreateSessionRow{}, ErrLoginFailed
	}
	if account.BlockedAt != nil || account.DeleteRequestedAt != nil {
		return CreateSessionRow{}, ErrAccountBlocked
	}
	if _, err = current.RecordGoogleLogin(ctx, account.ID); err != nil {
		return CreateSessionRow{}, fmt.Errorf("record Google login: %w", err)
	}
	p.Session.ReplacedHashes = p.ReplacedHashes
	p.Session.AccountID, p.Session.AuthEpoch = account.ID, account.AuthEpoch
	pending, err := current.primaryPending(ctx, p.Session, p.Pending, "google")
	if err != nil {
		return CreateSessionRow{}, err
	}
	if pending {
		if err = tx.Commit(ctx); err != nil {
			return CreateSessionRow{}, fmt.Errorf("commit Google pending authentication: %w", err)
		}
		return CreateSessionRow{}, &PendingRequired{CSRF: p.Session.CsrfSecret}
	}
	row, err := current.CreateSession(ctx, p.Session)
	if err != nil {
		return CreateSessionRow{}, fmt.Errorf("create Google session: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return row, fmt.Errorf("commit Google login: %w", err)
	}
	return row, nil
}
