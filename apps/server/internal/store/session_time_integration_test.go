//go:build integration

package store_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

func TestAccounts_SessionExpiryUsesTimeAfterLock(t *testing.T) {
	ctx, conn, _ := setup(t)
	id := webAccount(t, ctx, conn, "session-time")
	sessionID := webSession(t, ctx, conn, id, strings.Repeat("a", 64))
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = store.LockAccount(ctx, tx, id); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE web_session SET expires_at=transaction_timestamp()+interval '1 microsecond',last_seen_at=now()-interval '2 hours' WHERE id=$1`, sessionID); err != nil {
		t.Fatal(err)
	}
	var expired, now time.Time
	if err = tx.QueryRow(ctx, `SELECT expires_at,statement_timestamp() FROM web_session WHERE id=$1`, sessionID).Scan(&expired, &now); err != nil || !now.After(expired) {
		t.Fatal("fixture did not expire after transaction began")
	}
	q := store.New(tx)
	if _, err = q.GetLiveAuthoritySession(ctx, store.GetLiveAuthoritySessionParams{ID: sessionID, AccountID: id}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expired session authority=%v", err)
	}
	if count, err := q.TouchSession(ctx, strings.Repeat("a", 64)); err != nil || count != 0 {
		t.Fatalf("expired session slid: count=%d err=%v", count, err)
	}
}
