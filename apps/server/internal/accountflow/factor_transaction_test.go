package accountflow

import (
	"context"
	"errors"
	"testing"

	"danny.vn/trachuan/apps/server/internal/factor"
	"github.com/jackc/pgx/v5"
)

type factorTx struct {
	pgx.Tx
	committed bool
	commitErr error
}

func (tx *factorTx) Commit(context.Context) error   { tx.committed = true; return tx.commitErr }
func (tx *factorTx) Rollback(context.Context) error { return nil }

type factorDB struct {
	tx     *factorTx
	begins int
}

func (db *factorDB) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	db.begins++
	return db.tx, nil
}

func TestFactorTransaction_CommitsFailuresOnlyInCallerTransaction(t *testing.T) {
	for _, result := range []error{nil, factor.ErrFailed, &factor.Limited{}, ErrRateLimited, errors.New("synthetic write failure")} {
		tx := &factorTx{}
		db := &factorDB{tx: tx}
		s := Service{DB: db}
		err := s.transaction(t.Context(), func(actual pgx.Tx) error {
			if actual != tx {
				t.Fatal("different transaction")
			}
			return result
		})
		want := result == nil || errors.Is(result, factor.ErrFailed) || errors.Is(result, factor.ErrLimited)
		if tx.committed != want || db.begins != 1 || !errors.Is(err, result) {
			t.Fatalf("result %v committed=%v begins=%d err=%v", result, tx.committed, db.begins, err)
		}
	}
	tx := &factorTx{commitErr: errors.New("synthetic commit failure")}
	s := Service{DB: &factorDB{tx: tx}}
	if err := s.transaction(t.Context(), func(pgx.Tx) error { return factor.ErrFailed }); !errors.Is(err, tx.commitErr) {
		t.Fatal("lost commit failure", err)
	}
}
