//go:build integration

package accountflow

import (
	"testing"

	"danny.vn/trachuan/apps/server/internal/store"
)

func TestFactor_Retention(t *testing.T) {
	_, db, a := factorFixture(t)
	if _, err := db.Exec(t.Context(), `INSERT INTO pending_auth(token_hash,csrf_secret,account_id,auth_epoch,primary_kind,primary_at,expires_at,consumed_at) VALUES
 (repeat('1',64),'synthetic',$1,0,'google',now(),now()+interval '1 day',now()-interval '8 days'),
 (repeat('2',64),'synthetic',$1,0,'google',now(),now()-interval '8 days',NULL),
 (repeat('3',64),'synthetic',$1,0,'google',now(),now()-interval '6 days',NULL)`, a.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(t.Context(), `INSERT INTO totp_enrollment(account_id,web_session_id,auth_epoch,sealed_secret,key_id,expires_at) VALUES($1,$2,0,'synthetic','synthetic',now()-interval '8 days')`, a.AccountID, a.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(t.Context(), `INSERT INTO recovery_attempt(account_id,at) VALUES($1,now()-interval '2 days'),($1,now())`, a.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.New(db).PurgeEndedCredentials(t.Context()); err != nil {
		t.Fatal(err)
	}
	var pending, enrollments, attempts int
	if err := db.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM pending_auth),(SELECT count(*) FROM totp_enrollment),(SELECT count(*) FROM recovery_attempt)`).Scan(&pending, &enrollments, &attempts); err != nil || pending != 1 || enrollments != 0 || attempts != 1 {
		t.Fatalf("retention pending=%d enrollment=%d attempts=%d error=%v", pending, enrollments, attempts, err)
	}
}
