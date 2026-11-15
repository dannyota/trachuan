//go:build integration

package store_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
)

func TestRestoreFence_RevokesAllAndRepeatsSafely(t *testing.T) {
	ctx, conn, q, a, grant := oauthFixture(t)
	other := webAccount(t, ctx, conn, "other-restore")
	webSession(t, ctx, conn, a, strings.Repeat("a", 64))
	oauthCode(t, ctx, q, grant, strings.Repeat("b", 64), time.Now().Add(time.Hour))
	oauthToken(t, ctx, q, grant, strings.Repeat("c", 64), time.Now().Add(time.Hour))
	for _, sql := range []string{
		`INSERT INTO email_link(purpose,token_hash,email,expires_at) VALUES ('registration',repeat('f',64),'pending@example.test',now()+interval '1 hour')`,
		`INSERT INTO oauth_request(handle_hash,browser_hash,client_id,redirect_uri,state,code_challenge,resource,scope,expires_at) VALUES (repeat('d',64),repeat('e',64),'client','https://example.invalid/cb','synthetic','synthetic','https://example.invalid/mcp','mcp:read',now()+interval '5 minutes')`,
		`INSERT INTO login_transaction(handle_hash,state,nonce,pkce_verifier,next_path,redirect_uri,expires_at) VALUES (repeat('f',64),'synthetic','synthetic','synthetic','/','https://example.invalid/cb',now()+interval '5 minutes')`,
	} {
		if _, err := conn.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []int64{a, other} {
		for _, sql := range []string{
			`INSERT INTO personal_token(token_hash,account_id,name,expires_at,auth_epoch) VALUES (lpad(to_hex($1::bigint),64,'0'),$1,'synthetic',now()+interval '1 day',0)`,
			`INSERT INTO pending_auth(token_hash,csrf_secret,account_id,auth_epoch,primary_kind,primary_at,expires_at) VALUES (lpad(to_hex($1::bigint),64,'0'),'synthetic',$1,0,'google',now(),now()+interval '5 minutes')`,
			`INSERT INTO email_link(purpose,token_hash,email,account_id,auth_epoch,expires_at) VALUES ('reset',lpad(to_hex($1::bigint),64,'0'),'owner@example.test',$1,0,now()+interval '1 hour')`,
		} {
			if _, err := conn.Exec(ctx, sql, id); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := conn.Exec(ctx, `INSERT INTO totp_enrollment(account_id,web_session_id,sealed_secret,key_id,auth_epoch,expires_at) SELECT account_id,id,'synthetic','synthetic',0,now()+interval '10 minutes' FROM web_session WHERE account_id=$1`, a); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"fw-alpha", "fw-beta"} {
		if _, err := q.AllowFrameworkEntitlement(ctx, store.FrameworkAllowance{Email: "owner@example.test", Scope: scope, By: "operator", Note: "synthetic note"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := q.RevokeFrameworkEntitlement(ctx, store.FrameworkRevocation{Email: "owner@example.test", Scope: "fw-beta", By: "operator"}); err != nil {
		t.Fatal(err)
	}
	for run := 1; run <= 2; run++ {
		if err := q.RestoreFence(ctx); err != nil {
			t.Fatal(err)
		}
		var accounts int
		if err := conn.QueryRow(ctx, `SELECT count(*) FROM account WHERE auth_epoch=$1`, run).Scan(&accounts); err != nil || accounts != 2 {
			t.Fatalf("run=%d accounts=%d error=%v", run, accounts, err)
		}
		for _, table := range []string{"web_session", "personal_token", "oauth_grant", "oauth_code", "oauth_token", "framework_entitlement"} {
			var live int
			if err := conn.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM %s WHERE revoked_at IS NULL`, table)).Scan(&live); err != nil || live != 0 {
				t.Fatalf("%s live=%d error=%v", table, live, err)
			}
		}
		for _, table := range []string{"pending_auth", "totp_enrollment", "oauth_request", "login_transaction"} {
			var count int
			if err := conn.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM %s`, table)).Scan(&count); err != nil || count != 0 {
				t.Fatalf("%s rows=%d error=%v", table, count, err)
			}
		}
		var liveLinks, events, restoreEvents int
		if err := conn.QueryRow(ctx, `SELECT (SELECT count(*) FROM email_link WHERE consumed_at IS NULL),(SELECT count(*) FROM framework_entitlement_event),(SELECT count(*) FROM framework_entitlement_event WHERE action='revoke' AND reason='restore' AND actor='restore-fence' AND scope='fw-alpha' AND note='synthetic note')`).Scan(&liveLinks, &events, &restoreEvents); err != nil || liveLinks != 0 || events != 4 || restoreEvents != 1 {
			t.Fatalf("links=%d events=%d restore=%d error=%v", liveLinks, events, restoreEvents, err)
		}
	}
}

func TestRestoreFence_EventFailureRollsBackEverything(t *testing.T) {
	ctx, conn, q := setup(t)
	a, err := q.CreateAccount(ctx, store.CreateAccountParams{GoogleSub: "restore-subject", Email: "owner@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	webSession(t, ctx, conn, a.ID, strings.Repeat("a", 64))
	if _, err = q.AllowFrameworkEntitlement(ctx, store.FrameworkAllowance{Email: a.Email, Scope: "*", By: "operator"}); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, `CREATE FUNCTION reject_restore_event() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic failure'; END; $$; CREATE TRIGGER reject_restore BEFORE INSERT ON framework_entitlement_event FOR EACH ROW EXECUTE FUNCTION reject_restore_event()`); err != nil {
		t.Fatal(err)
	}
	if err = q.RestoreFence(ctx); err == nil {
		t.Fatal("restore succeeded without event")
	}
	var epoch, liveSessions, liveEntitlements, events int
	if err = conn.QueryRow(ctx, `SELECT auth_epoch,(SELECT count(*) FROM web_session WHERE revoked_at IS NULL),(SELECT count(*) FROM framework_entitlement WHERE revoked_at IS NULL),(SELECT count(*) FROM framework_entitlement_event) FROM account WHERE id=$1`, a.ID).Scan(&epoch, &liveSessions, &liveEntitlements, &events); err != nil || epoch != 0 || liveSessions != 1 || liveEntitlements != 1 || events != 1 {
		t.Fatalf("epoch=%d sessions=%d entitlements=%d events=%d error=%v", epoch, liveSessions, liveEntitlements, events, err)
	}
}
