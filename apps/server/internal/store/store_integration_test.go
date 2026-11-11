//go:build integration

package store_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
)

func setup(t *testing.T) (context.Context, *pgx.Conn, *store.Queries) {
	t.Helper()
	ctx := context.Background()
	url := testpg.NewDatabase(t)
	if err := migrate.Up(ctx, url); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })
	return ctx, conn, store.New(conn)
}

func TestStore_CredentialJoin(t *testing.T) {
	ctx, conn, q := setup(t)
	account, err := q.CreateAccount(ctx, store.CreateAccountParams{GoogleSub: "subject", Email: "test@example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	hash := strings.Repeat("a", 64)
	_, err = q.CreatePersonalToken(ctx, store.CreatePersonalTokenParams{TokenHash: hash, AccountID: account.ID, Name: "test", ExpiresAt: time.Now().Add(time.Hour), AuthEpoch: account.AuthEpoch})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `UPDATE account SET blocked_at = now(), auth_epoch = 2 WHERE id = $1`, account.ID); err != nil {
		t.Fatal(err)
	}
	credential, err := q.GetCredential(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	if credential.AccountID != account.ID || credential.BlockedAt == nil || credential.AccountAuthEpoch != 2 || credential.TokenAuthEpoch != 0 || credential.RevokedAt != nil {
		t.Fatalf("unexpected credential: %+v", credential)
	}
}

func TestStore_BlockRevokesTokensAndBumpsEpoch(t *testing.T) {
	ctx, conn, q := setup(t)
	account, err := q.CreateAccount(ctx, store.CreateAccountParams{GoogleSub: "subject", Email: "test@example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	for _, char := range []string{"a", "b"} {
		_, err := q.CreatePersonalToken(ctx, store.CreatePersonalTokenParams{TokenHash: strings.Repeat(char, 64), AccountID: account.ID, Name: char, ExpiresAt: time.Now().Add(time.Hour), AuthEpoch: account.AuthEpoch})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := q.BlockAccount(ctx, account.ID); err != nil {
		t.Fatal(err)
	}
	var epoch, revoked int
	var blocked *time.Time
	if err := conn.QueryRow(ctx, `SELECT auth_epoch, blocked_at, (SELECT count(*) FROM personal_token WHERE account_id = $1 AND revoked_at IS NOT NULL) FROM account WHERE id = $1`, account.ID).Scan(&epoch, &blocked, &revoked); err != nil {
		t.Fatal(err)
	}
	if epoch != 1 || blocked == nil || revoked != 2 {
		t.Fatalf("epoch=%d blocked=%v revoked=%d", epoch, blocked, revoked)
	}
}

func TestStore_ReleaseQueries(t *testing.T) {
	ctx, conn, q := setup(t)
	_, err := conn.Exec(ctx, `INSERT INTO corpus_release VALUES ('vn', 'active', now(), 'operator');
INSERT INTO retired_release (country, db_name, retired_at, retain_until, dropped_at) VALUES
('vn', 'retained', now(), now() + interval '1 day', NULL),
('vn', 'expired', now(), now() - interval '1 day', NULL),
('vn', 'dropped', now(), now() + interval '1 day', now()),
('xx', 'other', now(), now() + interval '1 day', NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	active, err := q.GetActiveRelease(ctx, "vn")
	if err != nil || active.ActiveDb != "active" {
		t.Fatalf("active=%+v err=%v", active, err)
	}
	retained, err := q.ListRetainedReleases(ctx, "vn")
	if err != nil || len(retained) != 1 || retained[0].DbName != "retained" {
		t.Fatalf("retained=%+v err=%v", retained, err)
	}
}
