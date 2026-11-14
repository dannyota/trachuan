//go:build integration

package store_test

import (
	"bytes"
	"encoding/base64"
	"testing"

	"danny.vn/trachuan/apps/server/internal/seal"
	"danny.vn/trachuan/apps/server/internal/store"
)

func TestKeys_ResealAndCheck(t *testing.T) {
	ctx, conn, provider := accountsMigration(t)
	if _, err := provider.UpTo(ctx, 5); err != nil {
		t.Fatal(err)
	}
	oldRaw := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	old, err := seal.Parse([]byte(oldRaw))
	if err != nil {
		t.Fatal(err)
	}
	ring, err := seal.Parse([]byte(base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32)) + "\n" + oldRaw))
	if err != nil {
		t.Fatal(err)
	}
	var account int64
	if err = conn.QueryRow(ctx, `INSERT INTO account(google_sub,email) VALUES ('synthetic','person@example.test') RETURNING id`).Scan(&account); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"totp_credential", "totp_enrollment"} {
		var id int64
		if err = conn.QueryRow(ctx, `SELECT nextval(pg_get_serial_sequence($1,'id'))`, table).Scan(&id); err != nil {
			t.Fatal(err)
		}
		purpose := "totp"
		if table == "totp_enrollment" {
			purpose = "totp_enrollment"
		}
		sealed, key, sealErr := old.Seal(seal.Binding{Purpose: purpose, OwnerKind: "account", OwnerID: account, RowID: id}, []byte("synthetic secret"))
		if sealErr != nil {
			t.Fatal(sealErr)
		}
		if table == "totp_credential" {
			_, err = conn.Exec(ctx, `INSERT INTO totp_credential(id,account_id,sealed_secret,key_id) OVERRIDING SYSTEM VALUE VALUES ($1,$2,$3,$4)`, id, account, sealed, key)
		} else {
			var session int64
			err = conn.QueryRow(ctx, `INSERT INTO web_session(token_hash,account_id,csrf_secret,auth_epoch,expires_at) VALUES (repeat('a',64),$1,'synthetic',0,now()+interval '1 day') RETURNING id`, account).Scan(&session)
			if err == nil {
				_, err = conn.Exec(ctx, `INSERT INTO totp_enrollment(id,account_id,web_session_id,sealed_secret,key_id,auth_epoch,expires_at) OVERRIDING SYSTEM VALUE VALUES ($1,$2,$3,$4,$5,0,now()+interval '1 hour')`, id, account, session, sealed, key)
			}
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	q := store.New(conn)
	counts, err := q.CheckAccountKeys(ctx, ring)
	if err != nil || counts["totp_credential"] != 1 || counts["totp_enrollment"] != 1 {
		t.Fatal("wrong old-key counts", err)
	}
	if err = q.ResealAccountKeys(ctx, ring); err != nil {
		t.Fatal(err)
	}
	counts, err = q.CheckAccountKeys(ctx, ring)
	if err != nil {
		t.Fatal(err)
	}
	for _, count := range counts {
		if count != 0 {
			t.Fatal("old key retained")
		}
	}
	if _, err = conn.Exec(ctx, `UPDATE totp_credential SET sealed_secret='broken'`); err != nil {
		t.Fatal(err)
	}
	if err = q.ResealAccountKeys(ctx, ring); err == nil {
		t.Fatal("reseal accepted corrupt row")
	}
}
