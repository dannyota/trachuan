package accountflow

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"

	"danny.vn/trachuan/apps/server/internal/factor"
	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

type FactorCompletion struct {
	TokenHash, CSRF, Code, RecoveryCode string
	UseRecovery                         bool
	Session                             store.CreateSessionParams
}

func (s *Service) PendingFactor(ctx context.Context, hash string) (store.PendingAuth, error) {
	db, ok := s.DB.(store.DBTX)
	if !ok {
		return store.PendingAuth{}, fmt.Errorf("pending store cannot query")
	}
	row, err := store.New(db).GetPendingAuth(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, factor.ErrFailed
	}
	if err != nil {
		return row, fmt.Errorf("read pending authentication: %w", err)
	}
	return row, nil
}
func (s *Service) CompleteFactor(ctx context.Context, p FactorCompletion) (store.CreateSessionRow, string, error) {
	var result store.CreateSessionRow
	var next string
	err := s.transaction(ctx, func(tx pgx.Tx) error {
		snapshot, err := store.New(tx).GetPendingAuth(ctx, p.TokenHash)
		if errors.Is(err, pgx.ErrNoRows) {
			return factor.ErrFailed
		}
		if err != nil {
			return fmt.Errorf("find pending owner: %w", err)
		}
		a, err := liveAccount(ctx, tx, Acting{AccountID: snapshot.AccountID, AuthEpoch: snapshot.AuthEpoch})
		if errors.Is(err, store.ErrUnauthorized) {
			return factor.ErrFailed
		}
		if err != nil {
			return err
		}
		pending, err := store.New(tx).LockPendingAuth(ctx, p.TokenHash)
		if errors.Is(err, pgx.ErrNoRows) {
			return factor.ErrFailed
		}
		if err != nil {
			return fmt.Errorf("lock pending authentication: %w", err)
		}
		if pending.AccountID != a.ID || pending.AuthEpoch != a.AuthEpoch || subtle.ConstantTimeCompare([]byte(p.CSRF), []byte(pending.CsrfSecret)) != 1 {
			return factor.ErrFailed
		}
		now, err := databaseNow(ctx, tx)
		if err != nil {
			return err
		}
		if err = s.verifyFactor(ctx, tx, a.ID, p.Code, recoveryInput(p.RecoveryCode, p.UseRecovery), now); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE pending_auth SET consumed_at=statement_timestamp() WHERE id=$1`, pending.ID); err != nil {
			return fmt.Errorf("consume pending authentication: %w", err)
		}
		p.Session.AccountID, p.Session.AuthEpoch = a.ID, a.AuthEpoch
		result, err = replacementSession(ctx, tx, p.Session, &pending.PrimaryAt, &now)
		if pending.Next != nil {
			next = *pending.Next
		} else {
			next = "/"
		}
		return err
	})
	return result, next, err
}
