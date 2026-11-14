package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

type deletionTx struct {
	pgx.Tx
	account                                    store.Account
	session                                    store.GetLiveAuthoritySessionRow
	live                                       bool
	locked, checked, bumped, marked, committed bool
}

func (tx *deletionTx) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	switch {
	case strings.Contains(query, "LockCredentialAccount"):
		tx.locked = true
		return authorityRow{value: tx.account}
	case strings.Contains(query, "GetLiveAuthoritySession"):
		if !tx.locked {
			return authorityRow{err: errors.New("deletion authority checked before account lock")}
		}
		tx.checked = true
		if !tx.live || args[0] != tx.session.ID || args[1] != tx.session.AccountID || args[2] != tx.session.AuthEpoch {
			return authorityRow{err: pgx.ErrNoRows}
		}
		return authorityRow{value: tx.session}
	case strings.Contains(query, "BumpAccountEpoch"):
		tx.bumped = true
		tx.account.AuthEpoch++
		return authorityRow{value: store.BumpAccountEpochRow(tx.account)}
	case strings.Contains(query, "MarkAccountDeleting"):
		tx.marked = true
		return authorityRow{value: tx.account}
	default:
		return authorityRow{err: errors.New("unexpected deletion query")}
	}
}
func (tx *deletionTx) Commit(context.Context) error { tx.committed = true; return nil }
func (*deletionTx) Rollback(context.Context) error  { return nil }

type deletionDB struct {
	store.DBTX
	tx *deletionTx
}

func (db deletionDB) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) { return db.tx, nil }

func TestRequestDeletion_RevalidatesActingSession(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name      string
		account   store.Account
		live      bool
		sessionID int64
		want      bool
	}{
		{"current", store.Account{ID: 1, AuthEpoch: 7}, true, 2, true},
		{"revoked", store.Account{ID: 1, AuthEpoch: 7}, false, 2, false},
		{"expired", store.Account{ID: 1, AuthEpoch: 7}, false, 2, false},
		{"wrong session", store.Account{ID: 1, AuthEpoch: 7}, true, 3, false},
		{"stale epoch", store.Account{ID: 1, AuthEpoch: 8}, true, 2, false},
		{"blocked", store.Account{ID: 1, AuthEpoch: 7, BlockedAt: &now}, true, 2, false},
		{"deleting", store.Account{ID: 1, AuthEpoch: 7, DeleteRequestedAt: &now}, true, 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := &deletionTx{account: tc.account, session: store.GetLiveAuthoritySessionRow{ID: tc.sessionID, AccountID: 1, AuthEpoch: 7}, live: tc.live}
			q := store.New(deletionDB{tx: tx})
			_, err := q.RequestDeletion(context.Background(), store.RequestDeletionParams{AccountID: 1, SessionID: 2, AuthEpoch: 7})
			if tc.want {
				if err != nil || !tx.checked || !tx.bumped || !tx.marked || !tx.committed {
					t.Fatalf("current deletion not authorized under lock: %v", err)
				}
			} else if !errors.Is(err, store.ErrUnauthorized) || tx.bumped || tx.marked || tx.committed {
				t.Fatalf("stale deletion changed state: error=%v bumped=%v marked=%v committed=%v", err, tx.bumped, tx.marked, tx.committed)
			}
		})
	}
}
