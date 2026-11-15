package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/admin"
	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

type blockStore struct {
	adminStore
	err error
}

func (s blockStore) BlockAccount(context.Context, int64) (store.BlockAccountRow, error) {
	return store.BlockAccountRow{}, s.err
}

type accountEvictions struct{ ids []int64 }

func (e *accountEvictions) EvictAccount(id int64) { e.ids = append(e.ids, id) }
func TestAdmin_BlockEvictsBothCaches(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "uncertain"}[failure], func(t *testing.T) {
			s := blockStore{}
			if failure {
				s.err = errors.New("uncertain")
			}
			sessions, tokens := &accountEvictions{}, &accountEvictions{}
			err := runAdmin(context.Background(), []string{"account", "block", "--account", "42"}, s, io.Discard, sessions, tokens)
			if (err != nil) != failure {
				t.Fatal(err)
			}
			for _, cache := range []*accountEvictions{sessions, tokens} {
				if len(cache.ids) != 1 || cache.ids[0] != 42 {
					t.Fatal("account cache not evicted")
				}
			}
		})
	}
}

type unblockStore struct {
	admin.Store
	account store.Account
	missing bool
}

func (s *unblockStore) MarkAccountUnblocked(_ context.Context, id int64) (store.Account, error) {
	if s.missing || s.account.ID != id || s.account.DeleteRequestedAt != nil {
		return store.Account{}, pgx.ErrNoRows
	}
	s.account.BlockedAt = nil
	return s.account, nil
}

func (s *unblockStore) GetAccount(_ context.Context, id int64) (store.Account, error) {
	if s.missing || s.account.ID != id {
		return store.Account{}, pgx.ErrNoRows
	}
	return s.account, nil
}

func TestAdmin_Unblock(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name              string
		deleting, missing bool
		wantError         string
	}{
		{name: "blocked"}, {name: "deleting", deleting: true, wantError: "delet"}, {name: "unknown", missing: true, wantError: "account"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &unblockStore{account: store.Account{ID: 42, BlockedAt: &now}, missing: tc.missing}
			if tc.deleting {
				s.account.DeleteRequestedAt = &now
			}
			sessions, tokens := &accountEvictions{}, &accountEvictions{}
			var out bytes.Buffer
			err := runAdmin(context.Background(), []string{"account", "unblock", "--account", "42"}, s, &out, sessions, tokens)
			if tc.wantError == "" {
				if err != nil || s.account.BlockedAt != nil || out.Len() != 0 {
					t.Fatalf("err=%v blocked=%v output=%q", err, s.account.BlockedAt, out.String())
				}
				if len(sessions.ids) != 1 || len(tokens.ids) != 1 {
					t.Fatal("caches not evicted")
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantError) || s.account.BlockedAt == nil {
				t.Fatalf("err=%v blocked=%v", err, s.account.BlockedAt)
			}
		})
	}
}
