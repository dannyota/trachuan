package accountflow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"danny.vn/trachuan/apps/server/internal/factor"
	"danny.vn/trachuan/apps/server/internal/seal"
	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

func recoveryInput(raw string, present bool) *string {
	if present || raw != "" {
		return &raw
	}
	return nil
}
func (s *Service) verifyFactor(ctx context.Context, tx pgx.Tx, id int64, code string, recovery *string, now time.Time) error {
	if recovery != nil {
		return verifyRecovery(ctx, tx, id, *recovery)
	}
	var row store.TotpCredential
	err := tx.QueryRow(ctx, `SELECT id,sealed_secret,key_id,last_step,failures,cooldown_until,last_failure_at FROM totp_credential WHERE account_id=$1 FOR UPDATE`, id).Scan(&row.ID, &row.SealedSecret, &row.KeyID, &row.LastStep, &row.Failures, &row.CooldownUntil, &row.LastFailureAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return factor.ErrFailed
	}
	if err != nil {
		return fmt.Errorf("read factor credential: %w", err)
	}
	if row.CooldownUntil != nil && row.CooldownUntil.After(now) {
		return &factor.Limited{Wait: row.CooldownUntil.Sub(now)}
	}
	secret, err := s.openFactor(ctx, tx, "totp_credential", "totp", row.ID, id, row.KeyID, row.SealedSecret)
	if err != nil {
		return err
	}
	state := factor.State{LastStep: row.LastStep, Failures: row.Failures, CooldownUntil: row.CooldownUntil, LastFailureAt: row.LastFailureAt}
	result := factor.Verify(secret, code, now, &state)
	if _, err = tx.Exec(ctx, `UPDATE totp_credential SET last_step=$2,failures=$3,cooldown_until=$4,last_failure_at=$5 WHERE id=$1`, row.ID, state.LastStep, state.Failures, state.CooldownUntil, state.LastFailureAt); err != nil {
		return fmt.Errorf("record factor verification: %w", err)
	}
	return result
}
func verifyRecovery(ctx context.Context, tx pgx.Tx, id int64, raw string) error {
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM recovery_attempt WHERE account_id=$1 AND at>statement_timestamp()-interval '1 day'`, id).Scan(&count); err != nil {
		return fmt.Errorf("count recovery failures: %w", err)
	}
	if count >= 10 {
		return &factor.Limited{Wait: 24 * time.Hour}
	}
	code, valid := factor.NormalizeRecovery(raw)
	if valid {
		result, err := tx.Exec(ctx, `UPDATE recovery_code SET used_at=statement_timestamp() WHERE account_id=$1 AND digest=$2 AND used_at IS NULL`, id, factor.RecoveryDigest(id, code))
		if err != nil {
			return fmt.Errorf("redeem recovery code: %w", err)
		}
		if result.RowsAffected() == 1 {
			return nil
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO recovery_attempt(account_id) VALUES($1)`, id); err != nil {
		return fmt.Errorf("record recovery failure: %w", err)
	}
	return factor.ErrFailed
}
func (s *Service) openFactor(ctx context.Context, tx pgx.Tx, table, purpose string, rowID, accountID int64, key string, data []byte) ([]byte, error) {
	binding := seal.Binding{Purpose: purpose, OwnerKind: "account", OwnerID: accountID, RowID: rowID}
	secret, previous, err := s.Ring.Open(binding, key, data)
	if err != nil || len(secret) != 20 {
		if s.Logger != nil {
			s.Logger.Error("totp_open_failed", "table", table, "row_id", rowID)
		}
		return nil, factor.ErrFailed
	}
	if previous {
		sealed, active, e := s.Ring.Seal(binding, secret)
		if e != nil {
			return nil, fmt.Errorf("reseal factor: %w", e)
		}
		query := fmt.Sprintf("UPDATE %s SET sealed_secret=$2,key_id=$3 WHERE id=$1", table)
		if _, err = tx.Exec(ctx, query, rowID, sealed, active); err != nil {
			return nil, fmt.Errorf("write resealed factor: %w", err)
		}
	}
	return secret, nil
}
