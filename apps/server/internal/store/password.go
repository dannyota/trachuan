package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type PasswordSnapshot = GetPasswordByEmailRow
type PasswordSessionParams struct {
	Verified PasswordSnapshot
	Session  CreateSessionParams
}

func (q *Queries) CreatePasswordSession(ctx context.Context, p PasswordSessionParams) (CreateSessionRow, error) {
	var session CreateSessionRow
	err := q.withPassword(ctx, p.Verified, func(current *Queries) error {
		enrolled, err := current.HasTOTP(ctx, p.Verified.AccountID)
		if err != nil {
			return fmt.Errorf("read password factor: %w", err)
		}
		if enrolled {
			return ErrLoginFailed
		}
		p.Session.AccountID = p.Verified.AccountID
		p.Session.AuthEpoch = p.Verified.AuthEpoch
		session, err = current.CreateSession(ctx, p.Session)
		if err != nil {
			return fmt.Errorf("create password session: %w", err)
		}
		return nil
	})
	return session, err
}
func (q *Queries) ReauthPassword(ctx context.Context, verified PasswordSnapshot, sessionID int64) error {
	return q.withPassword(ctx, verified, func(current *Queries) error {
		_, err := current.GetLiveAuthoritySession(ctx, GetLiveAuthoritySessionParams{ID: sessionID, AccountID: verified.AccountID, AuthEpoch: verified.AuthEpoch})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrLoginFailed
		}
		if err != nil {
			return fmt.Errorf("recheck password session: %w", err)
		}
		count, err := current.RecordPasswordProof(ctx, RecordPasswordProofParams{ID: sessionID, AccountID: verified.AccountID, AuthEpoch: verified.AuthEpoch})
		if err != nil {
			return fmt.Errorf("record password proof: %w", err)
		}
		if count != 1 {
			return ErrLoginFailed
		}
		return nil
	})
}
func (q *Queries) withPassword(ctx context.Context, verified PasswordSnapshot, write func(*Queries) error) error {
	if verified.Hash == nil || verified.Version == nil {
		return ErrLoginFailed
	}
	db, ok := q.db.(TransactionBeginner)
	if !ok {
		return fmt.Errorf("password store cannot begin transactions")
	}
	tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin password authority: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	account, err := LockAccount(ctx, tx, verified.AccountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrLoginFailed
	}
	if err != nil {
		return err
	}
	if account.BlockedAt != nil || account.DeleteRequestedAt != nil || account.AuthEpoch != verified.AuthEpoch {
		return ErrLoginFailed
	}
	current := New(tx)
	credential, err := current.GetPasswordCredential(ctx, account.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrLoginFailed
	}
	if err != nil {
		return fmt.Errorf("recheck password version: %w", err)
	}
	if credential.Version != *verified.Version || credential.Hash != *verified.Hash {
		return ErrLoginFailed
	}
	if err = write(current); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit password authority: %w", err)
	}
	return nil
}
