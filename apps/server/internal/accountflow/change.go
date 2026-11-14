package accountflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"danny.vn/trachuan/apps/server/internal/authmail"
	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

func (s *Service) CompleteReset(ctx context.Context, p Completion) (int64, error) {
	var accountID int64
	err := s.transaction(ctx, func(tx pgx.Tx) error {
		snapshot, err := readLink(ctx, tx, p.TokenHash, false)
		if err != nil {
			return err
		}
		if snapshot.Purpose != "reset" || snapshot.AccountID == nil || snapshot.AuthEpoch == nil {
			return ErrLinkInvalid
		}
		a, err := liveAccount(ctx, tx, Acting{AccountID: *snapshot.AccountID, AuthEpoch: *snapshot.AuthEpoch})
		if errors.Is(err, store.ErrUnauthorized) {
			return ErrLinkInvalid
		}
		if err != nil {
			return err
		}
		link, err := claim(ctx, tx, p.TokenHash, "reset", &a)
		if err != nil {
			return err
		}
		if _, err = store.New(tx).GetPasswordCredential(ctx, a.ID); errors.Is(err, pgx.ErrNoRows) {
			return ErrLinkInvalid
		}
		if err != nil {
			return fmt.Errorf("recheck reset password: %w", err)
		}
		if err = securityAllowance(ctx, tx, a.ID, "reset", 5); err != nil {
			return err
		}
		if err = consume(ctx, tx, link.ID); err != nil {
			return err
		}
		if err = s.setPassword(ctx, tx, a, p.Hash, false); err != nil {
			return err
		}
		accountID = a.ID
		return nil
	})
	return accountID, err
}
func (s *Service) ChangePassword(ctx context.Context, p Completion) (store.CreateSessionRow, error) {
	var result store.CreateSessionRow
	err := s.transaction(ctx, func(tx pgx.Tx) error {
		a, err := liveAccount(ctx, tx, p.Acting)
		if err != nil {
			return err
		}
		adding := p.TokenHash != ""
		acting, err := liveSession(ctx, tx, p.Acting, adding)
		if err != nil {
			return err
		}
		credential, err := store.New(tx).GetPasswordCredential(ctx, a.ID)
		if adding {
			if err == nil {
				return ErrInvalidRequest
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("read add credential: %w", err)
			}
			if _, err = claim(ctx, tx, p.TokenHash, "add_password", &a); err != nil {
				return err
			}
		} else {
			if errors.Is(err, pgx.ErrNoRows) {
				return store.ErrLoginFailed
			}
			if err != nil {
				return fmt.Errorf("recheck change password: %w", err)
			}
			v := p.Verified
			if v.AccountID != a.ID || v.AuthEpoch != a.AuthEpoch || v.Hash == nil || v.Version == nil || *v.Hash != credential.Hash || *v.Version != credential.Version {
				return store.ErrLoginFailed
			}
		}
		enrolled, err := store.New(tx).HasTOTP(ctx, a.ID)
		if err != nil {
			return fmt.Errorf("read password change factor: %w", err)
		}
		if enrolled {
			if p.CodeProvided || p.Code != "" {
				if err = s.verifyFactor(ctx, tx, a.ID, p.Code, nil, acting.CheckedAt); err != nil {
					return err
				}
				acting.FactorAt = &acting.CheckedAt
			} else if acting.FactorAt == nil || acting.FactorAt.Before(acting.CheckedAt.Add(-recentWindow)) {
				return store.ErrFactorRequired
			}
		}
		if err = securityAllowance(ctx, tx, a.ID, "signed_in", 10); err != nil {
			return err
		}
		if err = s.setPassword(ctx, tx, a, p.Hash, adding); err != nil {
			return err
		}
		p.Session.AccountID, p.Session.AuthEpoch = a.ID, a.AuthEpoch+1
		result, err = store.New(tx).CreateSession(ctx, p.Session)
		if err != nil {
			return fmt.Errorf("create password replacement session: %w", err)
		}
		proof := acting.ProofAt
		if !adding {
			proof = &acting.CheckedAt
		}
		if _, err = tx.Exec(ctx, `UPDATE web_session SET proof_at=$2,factor_at=$3 WHERE id=$1`, result.ID, proof, acting.FactorAt); err != nil {
			return fmt.Errorf("copy replacement proofs: %w", err)
		}
		result.ProofAt, result.FactorAt = proof, acting.FactorAt
		return nil
	})
	return result, err
}
func securityAllowance(ctx context.Context, tx pgx.Tx, id int64, kind string, limit int) error {
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM account_security_change WHERE account_id=$1 AND kind=$2 AND created_at>statement_timestamp()-interval '1 day'`, id, kind).Scan(&count); err != nil {
		return fmt.Errorf("count security changes: %w", err)
	}
	if count >= limit {
		return ErrRateLimited
	}
	if _, err := tx.Exec(ctx, `INSERT INTO account_security_change(account_id,kind) VALUES ($1,$2)`, id, kind); err != nil {
		return fmt.Errorf("record security change: %w", err)
	}
	return nil
}
func (s *Service) setPassword(ctx context.Context, tx pgx.Tx, a store.Account, hash string, adding bool) error {
	query := `UPDATE password_credential SET hash=$2,version=version+1 WHERE account_id=$1`
	if adding {
		query = `INSERT INTO password_credential(account_id,hash) VALUES ($1,$2)`
	}
	if _, err := tx.Exec(ctx, query, a.ID, hash); err != nil {
		return fmt.Errorf("set password: %w", err)
	}
	if _, err := store.BumpEpoch(ctx, tx, a.ID); err != nil {
		return err
	}
	payload, err := json.Marshal(authmail.Payload{To: a.Email})
	if err != nil {
		return fmt.Errorf("encode password notice: %w", err)
	}
	_, err = store.EnqueueMail(ctx, tx, s.Ring, store.EnqueueMailParams{Kind: "password_changed", Class: "notice", AccountID: &a.ID, Payload: payload})
	return err
}
