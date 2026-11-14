package accountflow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"danny.vn/trachuan/apps/server/internal/authmail"
	"danny.vn/trachuan/apps/server/internal/factor"
	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

type FactorChange struct {
	Acting             Acting
	Code, RecoveryCode string
	UseRecovery        bool
	Session            store.CreateSessionParams
}

func databaseNow(ctx context.Context, tx pgx.Tx) (time.Time, error) {
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT statement_timestamp()`).Scan(&now); err != nil {
		return now, fmt.Errorf("read factor time: %w", err)
	}
	return now, nil
}
func replacementSession(ctx context.Context, tx pgx.Tx, p store.CreateSessionParams, proof, factorAt *time.Time) (store.CreateSessionRow, error) {
	row, err := store.New(tx).CreateSession(ctx, p)
	if err != nil {
		return row, fmt.Errorf("create verified session: %w", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE web_session SET proof_at=$2,factor_at=$3 WHERE id=$1`, row.ID, proof, factorAt); err != nil {
		return store.CreateSessionRow{}, fmt.Errorf("record verified session proofs: %w", err)
	}
	row.ProofAt, row.FactorAt = proof, factorAt
	return row, nil
}
func (s *Service) factorNotice(ctx context.Context, tx pgx.Tx, a store.Account, kind string) error {
	payload, err := json.Marshal(authmail.Payload{To: a.Email})
	if err != nil {
		return fmt.Errorf("encode factor notice: %w", err)
	}
	_, err = store.EnqueueMail(ctx, tx, s.Ring, store.EnqueueMailParams{Kind: kind, Class: "notice", AccountID: &a.ID, Payload: payload})
	return err
}
func writeRecovery(ctx context.Context, tx pgx.Tx, id int64) ([]string, error) {
	codes, err := factor.NewRecoveryCodes()
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM recovery_code WHERE account_id=$1`, id); err != nil {
		return nil, fmt.Errorf("remove old recovery codes: %w", err)
	}
	for _, code := range codes {
		normalized, _ := factor.NormalizeRecovery(code)
		if _, err = tx.Exec(ctx, `INSERT INTO recovery_code(account_id,digest) VALUES($1,$2)`, id, factor.RecoveryDigest(id, normalized)); err != nil {
			return nil, fmt.Errorf("write recovery code: %w", err)
		}
	}
	return codes, nil
}
func (s *Service) RemoveTOTP(ctx context.Context, p FactorChange) (store.CreateSessionRow, error) {
	row, _, err := s.changeFactor(ctx, p, true)
	return row, err
}
func (s *Service) ReplaceRecoveryCodes(ctx context.Context, p FactorChange) (store.CreateSessionRow, []string, error) {
	return s.changeFactor(ctx, p, false)
}
func (s *Service) changeFactor(ctx context.Context, p FactorChange, remove bool) (store.CreateSessionRow, []string, error) {
	var result store.CreateSessionRow
	var codes []string
	err := s.transaction(ctx, func(tx pgx.Tx) error {
		a, err := liveAccount(ctx, tx, p.Acting)
		if err != nil {
			return err
		}
		acting, err := liveSession(ctx, tx, p.Acting, true)
		if err != nil {
			return err
		}
		now, err := databaseNow(ctx, tx)
		if err != nil {
			return err
		}
		if err = s.verifyFactor(ctx, tx, a.ID, p.Code, recoveryInput(p.RecoveryCode, p.UseRecovery), now); err != nil {
			return err
		}
		if err = securityAllowance(ctx, tx, a.ID, "signed_in", 10); err != nil {
			return err
		}
		kind := "recovery_codes_replaced"
		if remove {
			kind = "totp_removed"
			if _, err = tx.Exec(ctx, `DELETE FROM totp_credential WHERE account_id=$1`, a.ID); err != nil {
				return fmt.Errorf("remove factor: %w", err)
			}
			if _, err = tx.Exec(ctx, `DELETE FROM recovery_code WHERE account_id=$1`, a.ID); err != nil {
				return fmt.Errorf("remove recovery codes: %w", err)
			}
		} else {
			codes, err = writeRecovery(ctx, tx, a.ID)
			if err != nil {
				return err
			}
		}
		if _, err = store.BumpEpoch(ctx, tx, a.ID); err != nil {
			return err
		}
		if err = s.factorNotice(ctx, tx, a, kind); err != nil {
			return err
		}
		p.Session.AccountID, p.Session.AuthEpoch = a.ID, a.AuthEpoch+1
		result, err = replacementSession(ctx, tx, p.Session, acting.ProofAt, &now)
		return err
	})
	return result, codes, err
}
