//go:build integration

package store_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
)

func TestAccounts_BumpEpochRevokesEveryAuthority(t *testing.T) {
	ctx, conn, q, account, grant := oauthFixture(t)
	other := webAccount(t, ctx, conn, "other-epoch")
	webSession(t, ctx, conn, account, strings.Repeat("a", 64))
	oauthCode(t, ctx, q, grant, strings.Repeat("b", 64), time.Now().Add(time.Hour))
	oauthToken(t, ctx, q, grant, strings.Repeat("c", 64), time.Now().Add(time.Hour))
	for _, id := range []int64{account, other} {
		for _, sql := range []string{
			`INSERT INTO personal_token(token_hash,account_id,name,expires_at,auth_epoch) VALUES (lpad(to_hex($1::bigint),64,'0'),$1,'synthetic',now()+interval '1 day',0)`,
			`INSERT INTO pending_auth(token_hash,csrf_secret,account_id,auth_epoch,primary_kind,primary_at,expires_at) VALUES (lpad(to_hex($1::bigint),64,'0'),'synthetic',$1,0,'google',now(),now()+interval '5 minutes')`,
			`INSERT INTO email_link(purpose,token_hash,email,account_id,auth_epoch,expires_at) VALUES ('reset',lpad(to_hex($1::bigint),64,'0'),'owner@example.test',$1,0,now()+interval '1 hour')`,
			`INSERT INTO totp_credential(account_id,sealed_secret,key_id) VALUES ($1,'synthetic','synthetic')`,
			`INSERT INTO password_credential(account_id,hash,version) VALUES ($1,'synthetic',1)`,
			`INSERT INTO recovery_code(account_id,digest) VALUES ($1,lpad(to_hex($1::bigint),64,'0'))`,
		} {
			if _, err := conn.Exec(ctx, sql, id); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := conn.Exec(ctx, `INSERT INTO totp_enrollment(account_id,web_session_id,sealed_secret,key_id,auth_epoch,expires_at) SELECT account_id,id,'synthetic','synthetic',0,now()+interval '10 minutes' FROM web_session WHERE account_id=$1`, account); err != nil {
		t.Fatal(err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	evict, err := store.BumpEpoch(ctx, tx, account)
	if err != nil || evict != account {
		t.Fatalf("eviction account=%d err=%v", evict, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"web_session", "personal_token", "oauth_grant", "oauth_code", "oauth_token"} {
		var live int
		if err = conn.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM %s WHERE account_id=$1 AND revoked_at IS NULL`, table), account).Scan(&live); err != nil || live != 0 {
			t.Fatalf("%s live=%d err=%v", table, live, err)
		}
	}
	for _, table := range []string{"pending_auth", "totp_enrollment"} {
		var count int
		if err = conn.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM %s WHERE account_id=$1`, table), account).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s rows=%d err=%v", table, count, err)
		}
	}
	var links int
	if err = conn.QueryRow(ctx, `SELECT count(*) FROM email_link WHERE account_id=$1 AND consumed_at IS NULL`, account).Scan(&links); err != nil || links != 0 {
		t.Fatal("link remained live")
	}
	for _, table := range []string{"password_credential", "totp_credential", "recovery_code"} {
		var count int
		if err = conn.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM %s WHERE account_id IN ($1,$2)`, table), account, other).Scan(&count); err != nil || count != 2 {
			t.Fatalf("%s was changed", table)
		}
	}
	var untouched bool
	if err = conn.QueryRow(ctx, `SELECT a.auth_epoch=0 AND t.revoked_at IS NULL AND p.consumed_at IS NULL FROM account a JOIN personal_token t ON t.account_id=a.id JOIN pending_auth p ON p.account_id=a.id WHERE a.id=$1`, other).Scan(&untouched); err != nil || !untouched {
		t.Fatal("other account was revoked")
	}
	a, err := q.GetAccount(ctx, account)
	if err != nil || a.AuthEpoch != 1 {
		t.Fatal("epoch was not incremented")
	}
}
