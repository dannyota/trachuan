//go:build integration

package accountflow

import (
	"encoding/base32"
	"errors"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/factor"
	"danny.vn/trachuan/apps/server/internal/store"
)

func TestTOTP_EnrollmentAuthorityAndAttemptBound(t *testing.T) {
	for _, scenario := range []string{"other session", "expired", "stale epoch", "stale primary", "five failures"} {
		t.Run(scenario, func(t *testing.T) {
			s, db, a := factorFixture(t)
			setup, err := s.SetupTOTP(t.Context(), a)
			if err != nil {
				t.Fatal(err)
			}
			secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(setup.Secret)
			if err != nil {
				t.Fatal(err)
			}
			p := FactorChange{Acting: a, Code: factor.Code(secret, time.Now().Unix()/30), Session: factorSessionParams()}
			expected := error(factor.ErrFailed)
			switch scenario {
			case "other session":
				params := factorSessionParams()
				params.AccountID = a.AccountID
				other, e := store.New(db).CreateSession(t.Context(), params)
				if e != nil {
					t.Fatal(e)
				}
				p.Acting.SessionID = other.ID
			case "expired":
				_, err = db.Exec(t.Context(), `UPDATE totp_enrollment SET expires_at=now()`)
			case "stale epoch":
				_, err = store.New(db).BlockAccount(t.Context(), a.AccountID)
				expected = store.ErrUnauthorized
			case "stale primary":
				_, err = db.Exec(t.Context(), `UPDATE web_session SET proof_at=now()-interval '11 minutes'`)
				expected = store.ErrReauthRequired
			case "five failures":
				for i := range 5 {
					_, _, e := s.ConfirmTOTP(t.Context(), FactorChange{Acting: a, Code: "wrong", Session: factorSessionParams()})
					if !errors.Is(e, factor.ErrFailed) {
						t.Fatalf("failure %d: %v", i, e)
					}
				}
				var ended bool
				if e := db.QueryRow(t.Context(), `SELECT failures=5 AND expires_at<=statement_timestamp() FROM totp_enrollment WHERE account_id=$1`, a.AccountID).Scan(&ended); e != nil || !ended {
					t.Fatal("enrollment failure bound", e)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err = s.ConfirmTOTP(t.Context(), p); !errors.Is(err, expected) {
				t.Fatalf("got %v want %v", err, expected)
			}
			var count int
			if err = db.QueryRow(t.Context(), `SELECT count(*) FROM totp_credential`).Scan(&count); err != nil || count != 0 {
				t.Fatal("invalid enrollment installed credential", err)
			}
		})
	}
}
func TestTOTP_ReplacementKeepsRecoveryCodesAndRequiresRecentFactor(t *testing.T) {
	s, db, a := factorFixture(t)
	a, codes, _ := enroll(t, s, a)
	if _, err := db.Exec(t.Context(), `UPDATE web_session SET factor_at=now()-interval '11 minutes' WHERE id=$1`, a.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetupTOTP(t.Context(), a); !errors.Is(err, store.ErrFactorRequired) {
		t.Fatal("replacement skipped factor", err)
	}
	if _, err := db.Exec(t.Context(), `UPDATE web_session SET factor_at=now() WHERE id=$1`, a.SessionID); err != nil {
		t.Fatal(err)
	}
	setup, err := s.SetupTOTP(t.Context(), a)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(setup.Secret)
	if err != nil {
		t.Fatal(err)
	}
	row, newCodes, err := s.ConfirmTOTP(t.Context(), FactorChange{Acting: a, Code: factor.Code(secret, time.Now().Unix()/30), Session: factorSessionParams()})
	if err != nil || row.AuthEpoch != 2 || newCodes != nil {
		t.Fatal("replacement regenerated recovery codes", err)
	}
	normalized, _ := factor.NormalizeRecovery(codes[0])
	var kept bool
	if err = db.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM recovery_code WHERE account_id=$1 AND digest=$2 AND used_at IS NULL)`, a.AccountID, factor.RecoveryDigest(a.AccountID, normalized)).Scan(&kept); err != nil || !kept {
		t.Fatal("replacement lost recovery codes", err)
	}
}
