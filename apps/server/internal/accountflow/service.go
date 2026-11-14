package accountflow

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"danny.vn/trachuan/apps/server/internal/factor"
	"danny.vn/trachuan/apps/server/internal/seal"
	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

type failure string

func (f failure) Error() string { return string(f) }

const (
	ErrLinkInvalid    failure = "link_invalid"
	ErrRateLimited    failure = "rate_limited"
	ErrInvalidRequest failure = "invalid_request"
)

type Service struct {
	DB           store.TransactionBeginner
	Ring         *seal.Ring
	SignupEmails []string
	Logger       *slog.Logger
}
type Acting struct {
	AccountID, SessionID int64
	AuthEpoch            int32
}
type Completion struct {
	TokenHash, Hash string
	Acting          Acting
	Verified        store.PasswordSnapshot
	Session         store.CreateSessionParams
	Code            string
	CodeProvided    bool
}

func (s *Service) admits(email string) bool {
	return email != "" && (s.SignupEmails == nil || slices.Contains(s.SignupEmails, email))
}
func (s *Service) transaction(ctx context.Context, write func(pgx.Tx) error) error {
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin account flow: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	err = write(tx)
	if err != nil && !errors.Is(err, factor.ErrFailed) && !errors.Is(err, factor.ErrLimited) {
		return err
	}
	if commitErr := tx.Commit(ctx); commitErr != nil {
		return fmt.Errorf("commit account flow: %w", commitErr)
	}
	return err
}
func liveAccount(ctx context.Context, tx pgx.Tx, acting Acting) (store.Account, error) {
	a, err := store.LockAccount(ctx, tx, acting.AccountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, store.ErrUnauthorized
	}
	if err != nil {
		return a, err
	}
	if a.BlockedAt != nil || a.DeleteRequestedAt != nil || a.AuthEpoch != acting.AuthEpoch {
		return a, store.ErrUnauthorized
	}
	return a, nil
}
func liveSession(ctx context.Context, tx pgx.Tx, p Acting, recent bool) (store.GetLiveAuthoritySessionRow, error) {
	row, err := store.New(tx).GetLiveAuthoritySession(ctx, store.GetLiveAuthoritySessionParams{ID: p.SessionID, AccountID: p.AccountID, AuthEpoch: p.AuthEpoch})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, store.ErrUnauthorized
	}
	if err != nil {
		return row, fmt.Errorf("recheck acting session: %w", err)
	}
	if recent && (row.ProofAt == nil || row.ProofAt.Before(row.CheckedAt.Add(-recentWindow))) {
		return row, store.ErrReauthRequired
	}
	return row, nil
}
