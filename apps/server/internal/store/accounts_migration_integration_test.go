//go:build integration

package store_test

import (
	"encoding/json"
	"os"
	"testing"

	"danny.vn/trachuan/apps/server/internal/accountemail"
)

func TestAccounts_EmailParserAgreement(t *testing.T) {
	ctx, conn, _ := setup(t)
	data, err := os.ReadFile("../accountemail/testdata/addresses.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct{ Name, Raw, Want string }
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			want, parseErr := accountemail.Parse(tc.Raw)
			var got string
			err := conn.QueryRow(ctx, `SELECT account_email($1)`, tc.Raw).Scan(&got)
			if parseErr != nil {
				if err == nil {
					t.Fatal("SQL accepted rejected address")
				}
				return
			}
			if err != nil || got != want {
				t.Fatalf("SQL=%q error=%v want=%q", got, err, want)
			}
		})
	}
}

func TestAccounts_DeferredCredential(t *testing.T) {
	ctx, conn, _ := setup(t)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO account(email) VALUES ('missing@example.test')`); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err == nil {
		t.Fatal("credentialless account committed")
	}
	tx, err = conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	if err = tx.QueryRow(ctx, `INSERT INTO account(email) VALUES ('password@example.test') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO password_credential(account_id,hash,version) VALUES ($1,'synthetic',1)`, id); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	tx, err = conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `DELETE FROM password_credential WHERE account_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err == nil {
		t.Fatal("last credential deletion committed")
	}
	if _, err = conn.Exec(ctx, `DELETE FROM account WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
}
