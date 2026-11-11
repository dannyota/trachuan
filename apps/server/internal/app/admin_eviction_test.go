package app

import (
	"context"
	"errors"
	"io"
	"testing"

	"danny.vn/trachuan/apps/server/internal/admin"
	"danny.vn/trachuan/apps/server/internal/store"
)

type blockStore struct {
	admin.Store
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
