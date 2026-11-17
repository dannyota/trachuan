//go:build integration

package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

type failingAdminSession struct {
	credentialStore
}

func (s failingAdminSession) withAdminSession(ctx context.Context, id int64, epoch int32, write func(*store.Queries) error) error {
	return s.credentialStore.withAdminSession(ctx, id, epoch, func(q *store.Queries) error {
		if err := write(q); err != nil {
			return err
		}
		return errors.New("synthetic failure after writing session file")
	})
}

type failingSessionCommit struct{ transactionBeginner }

func (s failingSessionCommit) BeginTx(ctx context.Context, options pgx.TxOptions) (pgx.Tx, error) {
	tx, err := s.transactionBeginner.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	return failedSessionTx{Tx: tx}, nil
}

type failedSessionTx struct{ pgx.Tx }

func (failedSessionTx) Commit(context.Context) error {
	return errors.New("synthetic commit failure")
}

func TestAdminSession_FailureRollsBackAndRemovesFile(t *testing.T) {
	ctx, pool, q := adminSessionDatabase(t)
	idString := adminSessionAccount(t, ctx, "loadcheck:rollback")
	id, err := strconv.ParseInt(idString, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 50 {
		if _, err := q.CreateSession(ctx, store.CreateSessionParams{AccountID: id, TokenHash: fmt.Sprintf("%064x", i+1), CsrfSecret: "synthetic", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name string
		db   adminStore
	}{
		{name: "after file write", db: failingAdminSession{credentialStore: q}},
		{name: "commit", db: credentialStore{Queries: q.Queries, db: failingSessionCommit{transactionBeginner: pool}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "session")
			var out, stderr bytes.Buffer
			err := runAdmin(ctx, []string{"session", "create", "--account", idString, "--out", path}, tc.db, &out)
			if err == nil || err.Error() != "create session failed" || exitCode(err, &stderr) != 1 || out.Len() != 0 || stderr.String() != "server: create session failed\n" {
				t.Fatal("failure did not return a fixed diagnostic")
			}
			if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
				t.Fatal("failed command left its session file")
			}
			var total, revoked int
			if err := pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE revoked_at IS NOT NULL) FROM web_session WHERE account_id=$1`, id).Scan(&total, &revoked); err != nil || total != 50 || revoked != 0 {
				t.Fatal("failed command left a session or evicted an existing session")
			}
		})
	}
}

type changedSessionAccount struct {
	credentialStore
	change func(context.Context, int64) error
}

func (s changedSessionAccount) withAdminSession(ctx context.Context, id int64, epoch int32, write func(*store.Queries) error) error {
	if err := s.change(ctx, id); err != nil {
		return err
	}
	return s.credentialStore.withAdminSession(ctx, id, epoch, write)
}

func TestAdminSession_RechecksAccountUnderLock(t *testing.T) {
	ctx, pool, q := adminSessionDatabase(t)
	for _, tc := range []struct{ name, change string }{
		{name: "subject", change: `UPDATE account SET google_sub='real-google-subject' WHERE id=$1`},
		{name: "blocked", change: `UPDATE account SET blocked_at=now() WHERE id=$1`},
		{name: "deleting", change: `UPDATE account SET delete_requested_at=now() WHERE id=$1`},
		{name: "epoch", change: `UPDATE account SET auth_epoch=auth_epoch+1 WHERE id=$1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := adminSessionAccount(t, ctx, "loadcheck:race-"+tc.name)
			db := changedSessionAccount{credentialStore: q, change: func(ctx context.Context, accountID int64) error {
				_, err := pool.Exec(ctx, tc.change, accountID)
				return err
			}}
			path := filepath.Join(t.TempDir(), "session")
			var out bytes.Buffer
			if err := runAdmin(ctx, []string{"session", "create", "--account", id, "--out", path}, db, &out); err == nil {
				t.Fatal("changed account received a session")
			}
			if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
				t.Fatal("refusal left a file")
			}
			var count int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM web_session`).Scan(&count); err != nil || count != 0 {
				t.Fatal("changed account received a session row")
			}
		})
	}
}
