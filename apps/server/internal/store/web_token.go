package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const recentProofWindow = 10 * time.Minute

func (q *Queries) CreateWebPersonalToken(ctx context.Context, p WebPersonalTokenParams) (PersonalToken, error) {
	db, ok := q.db.(TransactionBeginner)
	if !ok {
		return PersonalToken{}, fmt.Errorf("token store cannot begin transactions")
	}
	var token PersonalToken
	err := WithAccount(ctx, db, p.Token.AccountID, p.Token.AuthEpoch, func(current *Queries) error {
		session, err := current.GetLiveAuthoritySession(ctx, GetLiveAuthoritySessionParams{ID: p.SessionID, AccountID: p.Token.AccountID, AuthEpoch: p.Token.AuthEpoch})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnauthorized
		}
		if err != nil {
			return fmt.Errorf("read token authority: %w", err)
		}
		if session.ProofAt == nil || session.ProofAt.Before(session.CheckedAt.Add(-recentProofWindow)) {
			return ErrReauthRequired
		}
		enrolled, err := current.HasTOTP(ctx, p.Token.AccountID)
		if err != nil {
			return fmt.Errorf("read token factor: %w", err)
		}
		if enrolled && (session.FactorAt == nil || session.FactorAt.Before(session.CheckedAt.Add(-recentProofWindow))) {
			return ErrFactorRequired
		}
		token, err = current.CreatePersonalToken(ctx, p.Token)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTokenLimit
		}
		return err
	})
	return token, err
}
