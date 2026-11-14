package accountflow

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"net/url"

	"danny.vn/trachuan/apps/server/internal/factor"
	"danny.vn/trachuan/apps/server/internal/seal"
	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

type TOTPSetup struct {
	URI    string `json:"otpauth_uri"`
	Secret string `json:"secret"`
}

func (s *Service) SetupTOTP(ctx context.Context, p Acting) (TOTPSetup, error) {
	var result TOTPSetup
	err := s.transaction(ctx, func(tx pgx.Tx) error {
		a, err := liveAccount(ctx, tx, p)
		if err != nil {
			return err
		}
		acting, err := liveSession(ctx, tx, p, true)
		if err != nil {
			return err
		}
		enrolled, err := store.New(tx).HasTOTP(ctx, a.ID)
		if err != nil {
			return fmt.Errorf("read setup factor: %w", err)
		}
		if enrolled && (acting.FactorAt == nil || acting.FactorAt.Before(acting.CheckedAt.Add(-recentWindow))) {
			return store.ErrFactorRequired
		}
		secret := make([]byte, 20)
		if _, err = rand.Read(secret); err != nil {
			return fmt.Errorf("generate TOTP secret: %w", err)
		}
		var id int64
		err = tx.QueryRow(ctx, `INSERT INTO totp_enrollment(account_id,web_session_id,sealed_secret,key_id,auth_epoch,expires_at)
   VALUES($1,$2,''::bytea,'',$3,statement_timestamp()+interval '10 minutes')
   ON CONFLICT(account_id) DO UPDATE SET web_session_id=EXCLUDED.web_session_id,sealed_secret=EXCLUDED.sealed_secret,key_id=EXCLUDED.key_id,
   auth_epoch=EXCLUDED.auth_epoch,failures=0,expires_at=EXCLUDED.expires_at RETURNING id`, a.ID, p.SessionID, a.AuthEpoch).Scan(&id)
		if err != nil {
			return fmt.Errorf("supersede TOTP enrollment: %w", err)
		}
		if err = s.sealFactor(ctx, tx, "totp_enrollment", "totp_enrollment", id, a.ID, secret); err != nil {
			return err
		}
		result.Secret = base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
		uri := url.URL{Scheme: "otpauth", Host: "totp", Path: "/trachuan.vn:" + a.Email}
		uri.RawQuery = url.Values{"secret": {result.Secret}, "issuer": {"trachuan.vn"}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}.Encode()
		result.URI = uri.String()
		return nil
	})
	return result, err
}
func (s *Service) ConfirmTOTP(ctx context.Context, p FactorChange) (store.CreateSessionRow, []string, error) {
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
		var enrollment store.TotpEnrollment
		err = tx.QueryRow(ctx, `SELECT id,web_session_id,auth_epoch,sealed_secret,key_id,failures,expires_at FROM totp_enrollment WHERE account_id=$1 FOR UPDATE`, a.ID).Scan(&enrollment.ID, &enrollment.WebSessionID, &enrollment.AuthEpoch, &enrollment.SealedSecret, &enrollment.KeyID, &enrollment.Failures, &enrollment.ExpiresAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return factor.ErrFailed
		}
		if err != nil {
			return fmt.Errorf("read TOTP enrollment: %w", err)
		}
		now, err := databaseNow(ctx, tx)
		if err != nil {
			return err
		}
		if enrollment.WebSessionID != p.Acting.SessionID || enrollment.AuthEpoch != a.AuthEpoch || !enrollment.ExpiresAt.After(now) || enrollment.Failures >= 5 {
			return factor.ErrFailed
		}
		secret, err := s.openFactor(ctx, tx, "totp_enrollment", "totp_enrollment", enrollment.ID, a.ID, enrollment.KeyID, enrollment.SealedSecret)
		if err != nil {
			return err
		}
		state := factor.State{}
		if err = factor.Verify(secret, p.Code, now, &state); err != nil {
			if _, writeErr := tx.Exec(ctx, `UPDATE totp_enrollment SET failures=failures+1,expires_at=CASE WHEN failures>=4 THEN statement_timestamp() ELSE expires_at END WHERE id=$1`, enrollment.ID); writeErr != nil {
				return fmt.Errorf("record enrollment failure: %w", writeErr)
			}
			return err
		}
		enrolled, err := store.New(tx).HasTOTP(ctx, a.ID)
		if err != nil {
			return fmt.Errorf("read enrollment credential: %w", err)
		}
		if err = securityAllowance(ctx, tx, a.ID, "signed_in", 10); err != nil {
			return err
		}
		var id int64
		err = tx.QueryRow(ctx, `INSERT INTO totp_credential(account_id,sealed_secret,key_id,last_step) VALUES($1,''::bytea,'',$2)
    ON CONFLICT(account_id) DO UPDATE SET last_step=EXCLUDED.last_step RETURNING id`, a.ID, state.LastStep).Scan(&id)
		if err != nil {
			return fmt.Errorf("install factor credential: %w", err)
		}
		if err = s.sealFactor(ctx, tx, "totp_credential", "totp", id, a.ID, secret); err != nil {
			return err
		}
		if !enrolled {
			codes, err = writeRecovery(ctx, tx, a.ID)
			if err != nil {
				return err
			}
		}
		if _, err = store.BumpEpoch(ctx, tx, a.ID); err != nil {
			return err
		}
		if err = s.factorNotice(ctx, tx, a, "totp_enrolled"); err != nil {
			return err
		}
		p.Session.AccountID, p.Session.AuthEpoch = a.ID, a.AuthEpoch+1
		result, err = replacementSession(ctx, tx, p.Session, acting.ProofAt, &now)
		return err
	})
	return result, codes, err
}
func (s *Service) sealFactor(ctx context.Context, tx pgx.Tx, table, purpose string, rowID, accountID int64, secret []byte) error {
	sealed, key, err := s.Ring.Seal(seal.Binding{Purpose: purpose, OwnerKind: "account", OwnerID: accountID, RowID: rowID}, secret)
	if err != nil {
		return fmt.Errorf("seal TOTP secret: %w", err)
	}
	query := fmt.Sprintf("UPDATE %s SET sealed_secret=$2,key_id=$3 WHERE id=$1", table)
	if _, err = tx.Exec(ctx, query, rowID, sealed, key); err != nil {
		return fmt.Errorf("store TOTP secret: %w", err)
	}
	return nil
}
