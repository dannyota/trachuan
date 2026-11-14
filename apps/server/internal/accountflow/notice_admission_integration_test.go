//go:build integration

package accountflow

import (
	"fmt"
	"testing"

	"danny.vn/trachuan/apps/server/internal/store"
)

func TestResetMail_AdmittedAfterTenSignedInPasswordChanges(t *testing.T) {
	s, db := fixture(t)
	acting, verified := actingFixture(t, s, db, true)
	for i := 0; i < 10; i++ {
		params := sessionParams()
		params.TokenHash = fmt.Sprintf("%064x", i+256)
		row, err := s.ChangePassword(t.Context(), Completion{Acting: acting, Verified: verified, Hash: fmt.Sprintf("synthetic hash %d", i), Session: params})
		if err != nil {
			t.Fatal(err)
		}
		acting.SessionID, acting.AuthEpoch = row.ID, row.AuthEpoch
		verified, err = store.New(db).GetPasswordByEmail(t.Context(), "person@example.test")
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RequestMail(t.Context(), "reset", "person@example.test"); err != nil {
		t.Fatal(err)
	}
	var notices, resetMail, links int
	err := db.QueryRow(t.Context(), `SELECT
  (SELECT count(*) FROM mail_outbox WHERE account_id=$1 AND kind='password_changed' AND class='notice'),
  (SELECT count(*) FROM mail_outbox WHERE account_id=$1 AND kind='reset' AND class='recovery' AND sealed_payload IS NOT NULL),
  (SELECT count(*) FROM email_link WHERE account_id=$1 AND purpose='reset' AND auth_epoch=10 AND consumed_at IS NULL AND expires_at>statement_timestamp())`, acting.AccountID).Scan(&notices, &resetMail, &links)
	if err != nil || notices != 10 || resetMail != 1 || links != 1 {
		t.Fatalf("notices=%d reset mail=%d live reset links=%d err=%v", notices, resetMail, links, err)
	}
}

func TestMailAdmission_ExcludesEverySecurityNoticeKind(t *testing.T) {
	for _, kind := range []string{"password_changed", "totp_enrolled", "totp_removed", "recovery_codes_replaced"} {
		t.Run(kind, func(t *testing.T) {
			s, db := fixture(t)
			acting, _ := actingFixture(t, s, db, true)
			if _, err := db.Exec(t.Context(), `INSERT INTO mail_outbox(kind,class,account_id,sealed_payload,key_id,expires_at)
    SELECT $1,'notice',$2,'synthetic','synthetic',statement_timestamp()+interval '1 day' FROM generate_series(1,10)`, kind, acting.AccountID); err != nil {
				t.Fatal(err)
			}
			if err := s.RequestMail(t.Context(), "reset", "person@example.test"); err != nil {
				t.Fatal(err)
			}
			var admitted int
			if err := db.QueryRow(t.Context(), `SELECT count(*) FROM mail_outbox WHERE account_id=$1 AND kind='reset' AND class='recovery' AND sealed_payload IS NOT NULL`, acting.AccountID).Scan(&admitted); err != nil || admitted != 1 {
				t.Fatal("security notices blocked reset mail", admitted, err)
			}
		})
	}
}
