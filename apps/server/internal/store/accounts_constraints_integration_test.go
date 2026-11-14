//go:build integration

package store_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestAccounts_EnumerationsAndOwnerChecks(t *testing.T) {
	ctx, conn, _ := setup(t)
	id := webAccount(t, ctx, conn, "constraints")
	for _, sql := range []string{
		`INSERT INTO email_link(purpose,token_hash,email,expires_at) VALUES ('unknown',repeat('a',64),'owner@example.test',now())`,
		`INSERT INTO email_link(purpose,token_hash,email,account_id,auth_epoch,expires_at) VALUES ('registration',repeat('a',64),'owner@example.test',$1,0,now())`,
		`INSERT INTO email_link(purpose,token_hash,email,expires_at) VALUES ('reset',repeat('a',64),'owner@example.test',now())`,
		`INSERT INTO email_link(purpose,token_hash,email,account_id,expires_at) VALUES ('reset',repeat('a',64),'owner@example.test',$1,now())`,
		`INSERT INTO email_link(purpose,token_hash,email,auth_epoch,expires_at) VALUES ('registration',repeat('a',64),'owner@example.test',0,now())`,
		`INSERT INTO email_link(purpose,token_hash,email,expires_at) VALUES ('registration','malformed','owner@example.test',now())`,
		`INSERT INTO pending_auth(token_hash,csrf_secret,account_id,auth_epoch,primary_kind,primary_at,expires_at) VALUES (repeat('a',64),'synthetic',$1,0,'unknown',now(),now())`,
		`INSERT INTO pending_auth(token_hash,csrf_secret,account_id,auth_epoch,primary_kind,primary_at,expires_at) VALUES ('malformed','synthetic',$1,0,'google',now(),now())`,
		`INSERT INTO recovery_code(account_id,digest) VALUES ($1,'malformed')`,
		`INSERT INTO mail_outbox(kind,class,account_id,key_id,expires_at) VALUES ('unknown','notice',$1,'synthetic',now())`,
		`INSERT INTO mail_outbox(kind,class,account_id,key_id,expires_at) VALUES ('reset','unknown',$1,'synthetic',now())`,
		`WITH link AS (INSERT INTO email_link(purpose,token_hash,email,expires_at) VALUES ('registration',repeat('b',64),'owner@example.test',now()) RETURNING id) INSERT INTO mail_outbox(kind,class,account_id,email_link_id,key_id,expires_at) SELECT 'registration','signup',$1,id,'synthetic',now() FROM link`,
		`INSERT INTO mail_outbox(kind,class,key_id,expires_at) VALUES ('registration','signup','synthetic',now())`,
	} {
		t.Run(sql, func(t *testing.T) {
			args := []any(nil)
			if strings.Contains(sql, "$1") {
				args = []any{id}
			}
			_, err := conn.Exec(ctx, sql, args...)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
				t.Fatalf("constraint error=%v", err)
			}
		})
	}
}

func TestAccounts_DeferredGoogleRemoval(t *testing.T) {
	ctx, conn, _ := setup(t)
	id := webAccount(t, ctx, conn, "primary")
	if _, err := conn.Exec(ctx, `UPDATE account SET google_sub=NULL WHERE id=$1`, id); err == nil {
		t.Fatal("removing last credential committed")
	}
	if _, err := conn.Exec(ctx, `INSERT INTO password_credential(account_id,hash,version) VALUES ($1,'synthetic',1)`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `UPDATE account SET google_sub=NULL WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
}
