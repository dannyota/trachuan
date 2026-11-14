//go:build integration

package accountflow

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/factor"
	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func factorSessionParams() store.CreateSessionParams {
	p := sessionParams()
	sum := sha256.Sum256([]byte(rand.Text()))
	p.TokenHash = hex.EncodeToString(sum[:])
	return p
}
func factorFixture(t *testing.T) (*Service, *pgxpool.Pool, Acting) {
	t.Helper()
	s, db := fixture(t)
	var id int64
	if err := db.QueryRow(t.Context(), `INSERT INTO account(google_sub,email) VALUES ('factor','factor@example.test') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	p := factorSessionParams()
	p.AccountID = id
	row, err := store.New(db).CreateSession(t.Context(), p)
	if err != nil {
		t.Fatal(err)
	}
	return s, db, Acting{AccountID: id, SessionID: row.ID}
}
func enroll(t *testing.T, s *Service, a Acting) (Acting, []string, []byte) {
	t.Helper()
	setup, err := s.SetupTOTP(t.Context(), a)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(setup.Secret)
	if err != nil {
		t.Fatal(err)
	}
	p := FactorChange{Acting: a, Code: factor.Code(secret, time.Now().Unix()/30), Session: factorSessionParams()}
	row, codes, err := s.ConfirmTOTP(t.Context(), p)
	if err != nil {
		t.Fatal(err)
	}
	a.SessionID = row.ID
	a.AuthEpoch = row.AuthEpoch
	return a, codes, secret
}
func TestTOTP_EnrollmentSupersedesAndBumpsEpoch(t *testing.T) {
	s, db, a := factorFixture(t)
	first, err := s.SetupTOTP(t.Context(), a)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.SetupTOTP(t.Context(), a)
	if err != nil {
		t.Fatal(err)
	}
	if first.Secret == second.Secret {
		t.Fatal("enrollment reused secret")
	}
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(second.Secret)
	if err != nil {
		t.Fatal(err)
	}
	row, codes, err := s.ConfirmTOTP(t.Context(), FactorChange{Acting: a, Code: factor.Code(secret, time.Now().Unix()/30), Session: factorSessionParams()})
	if err != nil || row.AuthEpoch != 1 || len(codes) != 10 || row.FactorAt == nil {
		t.Fatal("enrollment did not replace authority", err)
	}
	var revoked, notice bool
	if err = db.QueryRow(t.Context(), `SELECT revoked_at IS NOT NULL,EXISTS(SELECT 1 FROM mail_outbox WHERE account_id=$2 AND kind='totp_enrolled') FROM web_session WHERE id=$1`, a.SessionID, a.AccountID).Scan(&revoked, &notice); err != nil || !revoked || !notice {
		t.Fatal("enrollment side effects", err)
	}
}
func TestFactor_PendingBothPrimariesAndConcurrentRedemption(t *testing.T) {
	for _, kind := range []string{"password", "google"} {
		t.Run(kind, func(t *testing.T) {
			s, db, a := factorFixture(t)
			a, _, secret := enroll(t, s, a)
			if _, err := db.Exec(t.Context(), `INSERT INTO password_credential(account_id,hash) VALUES($1,'synthetic')`, a.AccountID); err != nil {
				t.Fatal(err)
			}
			q := store.New(db)
			p := factorSessionParams()
			p.TokenHash = strings.Repeat("d", 64)
			var err error
			if kind == "password" {
				v, e := q.GetPasswordByEmail(t.Context(), "factor@example.test")
				if e != nil {
					t.Fatal(e)
				}
				_, err = q.CreatePasswordSession(t.Context(), store.PasswordSessionParams{Verified: v, Session: p, Pending: store.PendingParams{Next: "/oauth/consent/test"}})
			} else {
				_, err = q.CreateGoogleSession(t.Context(), store.GoogleSessionParams{Identity: store.GoogleIdentity{Sub: "factor"}, Session: p, Pending: store.PendingParams{Next: "/oauth/consent/test"}})
			}
			var pending *store.PendingRequired
			if !errors.As(err, &pending) {
				t.Fatal("primary bypassed factor", err)
			}
			if _, err = db.Exec(t.Context(), `UPDATE totp_credential SET last_step=NULL WHERE account_id=$1`, a.AccountID); err != nil {
				t.Fatal(err)
			}
			var wg sync.WaitGroup
			out := make(chan error, 2)
			for i := range 2 {
				wg.Go(func() {
					replacement := factorSessionParams()
					replacement.TokenHash = strings.Repeat(string(rune('e'+i)), 64)
					_, next, e := s.CompleteFactor(t.Context(), FactorCompletion{TokenHash: p.TokenHash, CSRF: p.CsrfSecret, Code: factor.Code(secret, time.Now().Unix()/30), Session: replacement})
					if e == nil && next != "/oauth/consent/test" {
						e = errors.New("lost next")
					}
					out <- e
				})
			}
			wg.Wait()
			close(out)
			success := 0
			for e := range out {
				if e == nil {
					success++
				} else if !errors.Is(e, factor.ErrFailed) {
					t.Fatal(e)
				}
			}
			if success != 1 {
				t.Fatal("pending redeemed more than once")
			}
			var fresh bool
			if err = db.QueryRow(t.Context(), `SELECT factor_at IS NOT NULL AND proof_at=(SELECT primary_at FROM pending_auth WHERE token_hash=$2) FROM web_session WHERE account_id=$1 AND revoked_at IS NULL ORDER BY id DESC LIMIT 1`, a.AccountID, p.TokenHash).Scan(&fresh); err != nil || !fresh {
				t.Fatal("completion lost proofs", err)
			}
		})
	}
}
func TestFactor_RecoveryBoundAndRemovalNotice(t *testing.T) {
	s, db, a := factorFixture(t)
	a, codes, _ := enroll(t, s, a)
	if _, err := db.Exec(t.Context(), `UPDATE totp_credential SET cooldown_until=now()+interval '1 day' WHERE account_id=$1`, a.AccountID); err != nil {
		t.Fatal(err)
	}
	for i := range 11 {
		_, err := s.RemoveTOTP(t.Context(), FactorChange{Acting: a, RecoveryCode: "bad", Session: factorSessionParams()})
		if i < 10 && !errors.Is(err, factor.ErrFailed) || i == 10 && !errors.Is(err, factor.ErrLimited) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	var count int
	if err := db.QueryRow(t.Context(), `SELECT count(*) FROM recovery_attempt WHERE account_id=$1`, a.AccountID).Scan(&count); err != nil || count != 10 {
		t.Fatal("unbounded attempts", err)
	}
	if _, err := db.Exec(t.Context(), `UPDATE recovery_attempt SET at=now()-interval '2 days' WHERE account_id=$1`, a.AccountID); err != nil {
		t.Fatal(err)
	}
	row, err := s.RemoveTOTP(t.Context(), FactorChange{Acting: a, RecoveryCode: codes[0], Session: factorSessionParams()})
	if err != nil || row.AuthEpoch != 2 || row.FactorAt == nil {
		t.Fatal("recovery blocked by TOTP cooldown", err)
	}
	var removed, notice bool
	if err = db.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT 1 FROM totp_credential WHERE account_id=$1),EXISTS(SELECT 1 FROM mail_outbox WHERE account_id=$1 AND kind='totp_removed')`, a.AccountID).Scan(&removed, &notice); err != nil || !removed || !notice {
		t.Fatal("removal not atomic", err)
	}
}
func TestFactor_FailureCommitsAndReplacementNotice(t *testing.T) {
	s, db, a := factorFixture(t)
	a, old, secret := enroll(t, s, a)
	_, _, err := s.ReplaceRecoveryCodes(t.Context(), FactorChange{Acting: a, Code: "bad", Session: factorSessionParams()})
	if !errors.Is(err, factor.ErrFailed) {
		t.Fatal(err)
	}
	var failures int
	if err = db.QueryRow(t.Context(), `SELECT failures FROM totp_credential WHERE account_id=$1`, a.AccountID).Scan(&failures); err != nil || failures != 1 {
		t.Fatal("failure rolled back", err)
	}
	if _, err = db.Exec(t.Context(), `UPDATE totp_credential SET last_step=NULL WHERE account_id=$1`, a.AccountID); err != nil {
		t.Fatal(err)
	}
	row, codes, err := s.ReplaceRecoveryCodes(t.Context(), FactorChange{Acting: a, Code: factor.Code(secret, time.Now().Unix()/30), Session: factorSessionParams()})
	if err != nil || len(codes) != 10 || row.AuthEpoch != 2 {
		t.Fatal(err)
	}
	var oldPresent, notice bool
	n, _ := factor.NormalizeRecovery(old[0])
	if err = db.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM recovery_code WHERE digest=$2),EXISTS(SELECT 1 FROM mail_outbox WHERE account_id=$1 AND kind='recovery_codes_replaced')`, a.AccountID, factor.RecoveryDigest(a.AccountID, n)).Scan(&oldPresent, &notice); err != nil || oldPresent || !notice {
		t.Fatal("replacement did not invalidate old codes", err)
	}
}
