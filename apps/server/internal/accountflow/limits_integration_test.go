//go:build integration

package accountflow

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMailAdmission_GlobalCaps(t *testing.T) {
	for _, cap := range []string{"pending", "outbox"} {
		t.Run(cap, func(t *testing.T) {
			s, db := fixture(t)
			sql := `INSERT INTO email_link(purpose,token_hash,email,expires_at) SELECT 'registration',lpad(to_hex(g),64,'0'),'pending'||g||'@example.test',now()+interval '1 day' FROM generate_series(1,10000) g`
			if cap == "outbox" {
				sql = `WITH owner AS (INSERT INTO account(google_sub,email) VALUES ('synthetic','owner@example.test') RETURNING id)
     INSERT INTO mail_outbox(kind,class,account_id,key_id,expires_at) SELECT 'account_exists','signup',owner.id,'synthetic',now()+interval '1 day' FROM owner,generate_series(1,5000)`
			}
			if _, err := db.Exec(t.Context(), sql); err != nil {
				t.Fatal(err)
			}
			if err := s.RequestMail(t.Context(), "registration", "person@example.test"); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := db.QueryRow(t.Context(), `SELECT count(*) FROM email_link WHERE email='person@example.test'`).Scan(&count); err != nil || count != 0 {
				t.Fatal("global cap admitted link", err)
			}
		})
	}
}
func TestMailAdmission_RecipientWindows(t *testing.T) {
	s, db := fixture(t)
	for i := 0; i < 10; i++ {
		if i > 0 {
			if _, err := db.Exec(t.Context(), `UPDATE mail_outbox SET created_at=now()-interval '2 hours'`); err != nil {
				t.Fatal(err)
			}
		}
		requestLink(t, s, db, "registration", "person@example.test")
	}
	if err := s.RequestMail(t.Context(), "registration", "person@example.test"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(t.Context(), `SELECT count(*) FROM email_link`).Scan(&count); err != nil || count != 10 {
		t.Fatal("daily recipient cap exceeded", err)
	}
	if _, err := db.Exec(t.Context(), `UPDATE mail_outbox SET created_at=now()-interval '25 hours'`); err != nil {
		t.Fatal(err)
	}
	requestLink(t, s, db, "registration", "person@example.test")
	if err := db.QueryRow(t.Context(), `SELECT count(*) FROM email_link`).Scan(&count); err != nil || count != 11 {
		t.Fatal("daily recipient window did not renew", err)
	}
}
func actingFixture(t *testing.T, s *Service, db *pgxpool.Pool, password bool) (Acting, store.PasswordSnapshot) {
	t.Helper()
	q := store.New(db)
	a, err := q.CreateAccount(t.Context(), store.CreateAccountParams{GoogleSub: "synthetic", Email: "person@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if password {
		if _, err = db.Exec(t.Context(), `INSERT INTO password_credential(account_id,hash) VALUES ($1,'old')`, a.ID); err != nil {
			t.Fatal(err)
		}
	}
	p := sessionParams()
	p.AccountID = a.ID
	session, err := q.CreateSession(t.Context(), p)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := q.GetPasswordByEmail(t.Context(), a.Email)
	if err != nil {
		t.Fatal(err)
	}
	return Acting{AccountID: a.ID, SessionID: session.ID}, verified
}
func TestPasswordChange_AllowanceNoticeAndProofs(t *testing.T) {
	s, db := fixture(t)
	p, verified := actingFixture(t, s, db, true)
	for i := 0; i < 11; i++ {
		params := sessionParams()
		params.TokenHash = fmt.Sprintf("%064x", i+10)
		row, err := s.ChangePassword(t.Context(), Completion{Acting: p, Verified: verified, Hash: "new", Session: params})
		if i == 10 {
			if !errors.Is(err, ErrRateLimited) {
				t.Fatal("security allowance not enforced", err)
			}
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		var live int
		if err = db.QueryRow(t.Context(), `SELECT count(*) FROM web_session WHERE account_id=$1 AND revoked_at IS NULL`, p.AccountID).Scan(&live); err != nil || live != 1 {
			t.Fatal("replacement session count", err)
		}
		if row.ProofAt == nil || row.FactorAt != nil || row.ID == p.SessionID {
			t.Fatal("replacement proofs not set")
		}
		p.SessionID, p.AuthEpoch = row.ID, row.AuthEpoch
		verified, err = store.New(db).GetPasswordByEmail(t.Context(), "person@example.test")
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.QueryRow(t.Context(), `SELECT count(*) FROM mail_outbox WHERE class='notice'`).Scan(&count); err != nil || count != 10 {
		t.Fatal("not exactly one notice per change", err)
	}
	if _, err := db.Exec(t.Context(), `UPDATE account_security_change SET created_at=now()-interval '25 hours'`); err != nil {
		t.Fatal(err)
	}
	params := sessionParams()
	params.TokenHash = fmt.Sprintf("%064x", 100)
	if _, err := s.ChangePassword(t.Context(), Completion{Acting: p, Verified: verified, Hash: "new", Session: params}); err != nil {
		t.Fatal("security window did not renew", err)
	}
}
func TestAddPassword_RequiresOwnerRecentProofAndNoFactor(t *testing.T) {
	for _, refusal := range []string{"stale proof", "other owner", "enrolled", "already set", "expired", "stale epoch", "revoked session"} {
		t.Run(refusal, func(t *testing.T) {
			s, db := fixture(t)
			p, _ := actingFixture(t, s, db, false)
			if err := s.RequestAddPassword(t.Context(), p); err != nil {
				t.Fatal(err)
			}
			var digest string
			if err := db.QueryRow(t.Context(), `SELECT token_hash FROM email_link WHERE purpose='add_password'`).Scan(&digest); err != nil {
				t.Fatal(err)
			}
			want := ErrLinkInvalid
			var expected error = want
			switch refusal {
			case "stale proof":
				if _, err := db.Exec(t.Context(), `UPDATE web_session SET proof_at=now()-interval '11 minutes'`); err != nil {
					t.Fatal(err)
				}
				expected = store.ErrReauthRequired
			case "other owner":
				a, err := store.New(db).CreateAccount(t.Context(), store.CreateAccountParams{GoogleSub: "other", Email: "other@example.test"})
				if err != nil {
					t.Fatal(err)
				}
				params := sessionParams()
				params.TokenHash = strings.Repeat("b", 64)
				params.AccountID = a.ID
				session, err := store.New(db).CreateSession(t.Context(), params)
				if err != nil {
					t.Fatal(err)
				}
				p.AccountID, p.SessionID = a.ID, session.ID
			case "enrolled":
				if _, err := db.Exec(t.Context(), `INSERT INTO totp_credential(account_id,sealed_secret,key_id) VALUES ($1,'synthetic','synthetic')`, p.AccountID); err != nil {
					t.Fatal(err)
				}
				expected = store.ErrFactorRequired
			case "already set":
				if _, err := db.Exec(t.Context(), `INSERT INTO password_credential(account_id,hash) VALUES ($1,'old')`, p.AccountID); err != nil {
					t.Fatal(err)
				}
				expected = ErrInvalidRequest
			case "expired":
				if _, err := db.Exec(t.Context(), `UPDATE email_link SET expires_at=now()`); err != nil {
					t.Fatal(err)
				}
			case "stale epoch":
				if _, err := db.Exec(t.Context(), `UPDATE account SET auth_epoch=auth_epoch+1 WHERE id=$1`, p.AccountID); err != nil {
					t.Fatal(err)
				}
				expected = store.ErrUnauthorized
			case "revoked session":
				if _, err := db.Exec(t.Context(), `UPDATE web_session SET revoked_at=now()`); err != nil {
					t.Fatal(err)
				}
				expected = store.ErrUnauthorized
			}
			_, err := s.ChangePassword(t.Context(), Completion{Acting: p, TokenHash: digest, Hash: "new", Session: sessionParams()})
			if !errors.Is(err, expected) {
				t.Fatalf("got %v want %v", err, expected)
			}
			var notices int
			if err = db.QueryRow(t.Context(), `SELECT count(*) FROM mail_outbox WHERE class='notice'`).Scan(&notices); err != nil || notices != 0 {
				t.Fatal("refusal wrote notice", err)
			}
		})
	}
}
func TestLinks_RetentionAndExpiry(t *testing.T) {
	s, db := fixture(t)
	digest := requestLink(t, s, db, "registration", "person@example.test")
	if _, err := db.Exec(t.Context(), `UPDATE email_link SET expires_at=now()`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LookupLink(t.Context(), digest); !errors.Is(err, ErrLinkInvalid) {
		t.Fatal("expired link visible", err)
	}
	if _, err := db.Exec(t.Context(), `INSERT INTO email_link(purpose,token_hash,email,expires_at,consumed_at) VALUES
 ('registration',repeat('a',64),'old@example.test',now()+interval '1 day',now()-interval '8 days'),
 ('registration',repeat('b',64),'expired@example.test',now()-interval '8 days',NULL),
 ('registration',repeat('c',64),'retained@example.test',now()-interval '6 days',NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.New(db).PurgeEndedCredentials(t.Context()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(t.Context(), `SELECT count(*) FROM email_link`).Scan(&count); err != nil || count != 2 {
		t.Fatal("wrong link retention", count, err)
	}
	if _, err := s.CompleteRegistration(t.Context(), Completion{TokenHash: digest, Hash: "new", Session: sessionParams()}); !errors.Is(err, ErrLinkInvalid) {
		t.Fatal("expired link completed", err)
	}
}

func TestAddPassword_CopiesProofsAndRevokesActingSession(t *testing.T) {
	s, db := fixture(t)
	acting, _ := actingFixture(t, s, db, false)
	var proof, factor time.Time
	if err := db.QueryRow(t.Context(), `UPDATE web_session SET proof_at=statement_timestamp()-interval '5 minutes',factor_at=statement_timestamp()-interval '8 minutes' WHERE id=$1 RETURNING proof_at,factor_at`, acting.SessionID).Scan(&proof, &factor); err != nil {
		t.Fatal(err)
	}
	if err := s.RequestAddPassword(t.Context(), acting); err != nil {
		t.Fatal(err)
	}
	var digest string
	if err := db.QueryRow(t.Context(), `SELECT token_hash FROM email_link WHERE purpose='add_password'`).Scan(&digest); err != nil {
		t.Fatal(err)
	}
	params := sessionParams()
	params.TokenHash = strings.Repeat("d", 64)
	result, err := s.ChangePassword(t.Context(), Completion{Acting: acting, TokenHash: digest, Hash: "new", Session: params})
	if err != nil {
		t.Fatal(err)
	}
	if result.ProofAt == nil || !result.ProofAt.Equal(proof) || result.FactorAt == nil || !result.FactorAt.Equal(factor) {
		t.Fatal("add password advanced or lost proofs")
	}
	var valid bool
	if err = db.QueryRow(t.Context(), `SELECT a.google_sub='synthetic' AND a.auth_epoch=1 AND p.version=1 AND old.revoked_at IS NOT NULL AND n.revoked_at IS NULL
 FROM account a JOIN password_credential p ON p.account_id=a.id JOIN web_session old ON old.id=$2 JOIN web_session n ON n.id=$3 WHERE a.id=$1`, acting.AccountID, acting.SessionID, result.ID).Scan(&valid); err != nil || !valid {
		t.Fatal("add password authority mismatch", err)
	}
}
func TestReset_PreservesFactorAndRecoveryState(t *testing.T) {
	s, db := fixture(t)
	acting, _ := actingFixture(t, s, db, true)
	if _, err := db.Exec(t.Context(), `WITH factor AS (INSERT INTO totp_credential(account_id,sealed_secret,key_id,last_step,failures,cooldown_until,last_failure_at) VALUES ($1,'synthetic','synthetic',123,7,now()+interval '1 day',now())) INSERT INTO recovery_code(account_id,digest) VALUES ($1,repeat('a',64))`, acting.AccountID); err != nil {
		t.Fatal(err)
	}
	var before string
	if err := db.QueryRow(t.Context(), `SELECT row_to_json(t)::text FROM totp_credential t WHERE account_id=$1`, acting.AccountID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	digest := requestLink(t, s, db, "reset", "person@example.test")
	if _, err := s.CompleteReset(t.Context(), Completion{TokenHash: digest, Hash: "new"}); err != nil {
		t.Fatal(err)
	}
	var after string
	var codes, live int
	if err := db.QueryRow(t.Context(), `SELECT row_to_json(t)::text,(SELECT count(*) FROM recovery_code WHERE account_id=$1 AND used_at IS NULL),(SELECT count(*) FROM web_session WHERE account_id=$1 AND revoked_at IS NULL) FROM totp_credential t WHERE account_id=$1`, acting.AccountID).Scan(&after, &codes, &live); err != nil || after != before || codes != 1 || live != 0 {
		t.Fatal("reset changed factor or kept a session", err)
	}
	if _, err := s.CompleteReset(t.Context(), Completion{TokenHash: digest, Hash: "new"}); !errors.Is(err, ErrLinkInvalid) {
		t.Fatal("reset link replay accepted", err)
	}
}
