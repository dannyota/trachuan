//go:build integration

package accountflow

import (
	"errors"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/factor"
	"danny.vn/trachuan/apps/server/internal/store"
)

func TestPasswordChange_EnrolledFactorRules(t *testing.T) {
	for _, scenario := range []string{"recent", "stale missing", "stale code", "stale wrong", "empty code", "cooldown"} {
		t.Run(scenario, func(t *testing.T) {
			s, db, a := factorFixture(t)
			a, _, secret := enroll(t, s, a)
			if _, err := db.Exec(t.Context(), `INSERT INTO password_credential(account_id,hash) VALUES($1,'old')`, a.AccountID); err != nil {
				t.Fatal(err)
			}
			verified, err := store.New(db).GetPasswordByEmail(t.Context(), "factor@example.test")
			if err != nil {
				t.Fatal(err)
			}
			p := Completion{Acting: a, Verified: verified, Hash: "new", Session: factorSessionParams()}
			var expected error
			if scenario != "recent" {
				if _, err = db.Exec(t.Context(), `UPDATE web_session SET factor_at=now()-interval '11 minutes' WHERE id=$1`, a.SessionID); err != nil {
					t.Fatal(err)
				}
			}
			switch scenario {
			case "stale missing":
				expected = store.ErrFactorRequired
			case "stale code":
				p.Code = factor.Code(secret, time.Now().Unix()/30)
			case "stale wrong":
				p.Code = "wrong"
				expected = factor.ErrFailed
			case "empty code":
				p.CodeProvided = true
				expected = factor.ErrFailed
			case "cooldown":
				p.Code = "wrong"
				expected = factor.ErrLimited
			}
			if _, err = db.Exec(t.Context(), `UPDATE totp_credential SET last_step=NULL WHERE account_id=$1`, a.AccountID); err != nil {
				t.Fatal(err)
			}
			if scenario == "cooldown" {
				if _, err = db.Exec(t.Context(), `UPDATE totp_credential SET cooldown_until=now()+interval '15 minutes' WHERE account_id=$1`, a.AccountID); err != nil {
					t.Fatal(err)
				}
			}
			row, err := s.ChangePassword(t.Context(), p)
			if !errors.Is(err, expected) {
				t.Fatalf("got %v want %v", err, expected)
			}
			if expected == nil && (row.AuthEpoch != 2 || row.FactorAt == nil || row.ProofAt == nil) {
				t.Fatal("password change lost proofs")
			}
			var hash string
			var failures int
			if err = db.QueryRow(t.Context(), `SELECT p.hash,t.failures FROM password_credential p JOIN totp_credential t ON t.account_id=p.account_id WHERE p.account_id=$1`, a.AccountID).Scan(&hash, &failures); err != nil {
				t.Fatal(err)
			}
			if expected != nil && hash != "old" || (scenario == "stale wrong" || scenario == "empty code") && failures != 1 || scenario == "cooldown" && failures != 0 {
				t.Fatal("failure mutated password or lost verifier state")
			}
		})
	}
}
func TestAddPassword_EnrolledRecentFactor(t *testing.T) {
	s, db, a := factorFixture(t)
	a, _, _ = enroll(t, s, a)
	if err := s.RequestAddPassword(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	var token string
	if err := db.QueryRow(t.Context(), `SELECT token_hash FROM email_link WHERE account_id=$1 AND purpose='add_password' AND consumed_at IS NULL`, a.AccountID).Scan(&token); err != nil {
		t.Fatal(err)
	}
	row, err := s.ChangePassword(t.Context(), Completion{Acting: a, TokenHash: token, Hash: "new", Session: factorSessionParams()})
	if err != nil || row.AuthEpoch != 2 || row.FactorAt == nil {
		t.Fatal("recent factor refused", err)
	}
}
