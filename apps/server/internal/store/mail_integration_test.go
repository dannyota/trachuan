//go:build integration

package store_test

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/seal"
	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

func TestMail_SealingClaimAndRotation(t *testing.T) {
	ctx, conn, provider := accountsMigration(t)
	if _, err := provider.UpTo(ctx, 6); err != nil {
		t.Fatal(err)
	}
	var accountID int64
	if err := conn.QueryRow(ctx, `INSERT INTO account(google_sub,email) VALUES ('synthetic','person@example.test') RETURNING id`).Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	old, err := seal.Parse([]byte(base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))))
	if err != nil {
		t.Fatal(err)
	}
	ring, err := seal.Parse([]byte(base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32)) + "\n" + base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"to":"person@example.test","token":"synthetic"}`)
	id, err := store.EnqueueMail(ctx, tx, old, store.EnqueueMailParams{Kind: "reset", Class: "recovery", AccountID: &accountID, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	var sealed []byte
	if err = tx.QueryRow(ctx, `SELECT sealed_payload FROM mail_outbox WHERE id=$1`, id).Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("person@example.test")) || bytes.Contains(sealed, []byte("synthetic")) {
		t.Fatal("unsealed payload stored")
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	q := store.New(conn)
	now := time.Now().UTC().Truncate(time.Microsecond)
	row, err := q.ClaimDueMail(ctx, now)
	if err != nil || row.Attempts != 1 {
		t.Fatal("claim failed", err)
	}
	if _, err = q.ClaimDueMail(ctx, now); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("active claim stolen")
	}
	plain, err := q.OpenClaimedMail(ctx, ring, row)
	if err != nil || !bytes.Equal(plain, payload) {
		t.Fatal("payload failed to open", err)
	}
	var keyID string
	if err = conn.QueryRow(ctx, `SELECT key_id FROM mail_outbox WHERE id=$1`, id).Scan(&keyID); err != nil || keyID != ring.ActiveID() {
		t.Fatal("previous key not resealed")
	}
	reclaimed, err := q.ClaimDueMail(ctx, now.Add(2*time.Minute+time.Second))
	if err != nil || reclaimed.Attempts != 2 {
		t.Fatal("crash claim not reclaimed", err)
	}
	if err = q.FinishMail(ctx, store.FinishMailParams{ID: id, Attempts: row.Attempts, Now: now, Sent: true}); err != nil {
		t.Fatal(err)
	}
	var sent *time.Time
	if err = conn.QueryRow(ctx, `SELECT sent_at FROM mail_outbox WHERE id=$1`, id).Scan(&sent); err != nil || sent != nil {
		t.Fatal("stale sender changed row")
	}
	if err = q.FinishMail(ctx, store.FinishMailParams{ID: id, Attempts: reclaimed.Attempts, Now: now, Sent: true}); err != nil {
		t.Fatal(err)
	}
	if err = conn.QueryRow(ctx, `SELECT sealed_payload FROM mail_outbox WHERE id=$1`, id).Scan(&sealed); err != nil || sealed != nil {
		t.Fatal("sent payload retained")
	}
}

func TestMail_BackoffExhaustionAndRetention(t *testing.T) {
	ctx, conn, provider := accountsMigration(t)
	if _, err := provider.UpTo(ctx, 6); err != nil {
		t.Fatal(err)
	}
	var accountID int64
	if err := conn.QueryRow(ctx, `INSERT INTO account(google_sub,email) VALUES ('synthetic','person@example.test') RETURNING id`).Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	ring, err := seal.Parse([]byte(base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.EnqueueMail(ctx, tx, ring, store.EnqueueMailParams{Kind: "password_changed", Class: "notice", AccountID: &accountID, Payload: []byte(`{"to":"person@example.test"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	q := store.New(conn)
	now := time.Now().UTC().Truncate(time.Microsecond)
	for attempt := int32(1); attempt <= 6; attempt++ {
		row, claimErr := q.ClaimDueMail(ctx, now)
		if claimErr != nil || row.Attempts != attempt {
			t.Fatal("retry claim failed", claimErr)
		}
		if err = q.FinishMail(ctx, store.FinishMailParams{ID: id, Attempts: attempt, Now: now}); err != nil {
			t.Fatal(err)
		}
		var next time.Time
		var payload []byte
		if err = conn.QueryRow(ctx, `SELECT next_attempt_at,sealed_payload FROM mail_outbox WHERE id=$1`, id).Scan(&next, &payload); err != nil {
			t.Fatal(err)
		}
		delay := []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 16 * time.Minute, 32 * time.Minute}[attempt-1]
		if !next.Equal(now.Add(delay)) {
			t.Fatal("wrong retry backoff")
		}
		if (payload == nil) != (attempt == 6) {
			t.Fatal("wrong payload exhaustion")
		}
		if _, err = q.ClaimDueMail(ctx, now.Add(delay-time.Microsecond)); err != pgx.ErrNoRows {
			t.Fatal("retry claimed early")
		}
		now = next
	}
	if _, err = conn.Exec(ctx, `UPDATE mail_outbox SET expires_at=now()-interval '8 days' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if count, purgeErr := q.PurgeMail(ctx); purgeErr != nil || count != 1 {
		t.Fatal("expired row not purged", purgeErr)
	}
}

func TestMail_EmailLinkBindingAndExpiry(t *testing.T) {
	ctx, conn, provider := accountsMigration(t)
	if _, err := provider.UpTo(ctx, 6); err != nil {
		t.Fatal(err)
	}
	var linkID int64
	if err := conn.QueryRow(ctx, `INSERT INTO email_link(purpose,token_hash,email,expires_at) VALUES ('registration',repeat('a',64),'person@example.test',now()+interval '1 day') RETURNING id`).Scan(&linkID); err != nil {
		t.Fatal(err)
	}
	ring, err := seal.Parse([]byte(base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.EnqueueMail(ctx, tx, ring, store.EnqueueMailParams{Kind: "registration", Class: "signup", EmailLinkID: &linkID, Payload: []byte(`{"to":"person@example.test","token":"synthetic"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	q := store.New(conn)
	row, err := q.ClaimDueMail(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	binding := store.MailBinding(row)
	if binding.OwnerKind != "email_link" || binding.OwnerID != linkID || binding.RowID != id || binding.Purpose != "mail" {
		t.Fatal("wrong registration binding")
	}
	if _, err = q.OpenClaimedMail(ctx, ring, row); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, `UPDATE mail_outbox SET expires_at=now()-interval '1 second' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = q.PurgeMail(ctx); err != nil {
		t.Fatal(err)
	}
	var payload []byte
	if err = conn.QueryRow(ctx, `SELECT sealed_payload FROM mail_outbox WHERE id=$1`, id).Scan(&payload); err != nil || payload != nil {
		t.Fatal("expired payload retained")
	}
}
