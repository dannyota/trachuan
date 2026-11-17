package store_test

import (
	"context"
	"errors"
	"testing"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

type reservedGoogleDB struct {
	store.DBTX
	began bool
}

func (db *reservedGoogleDB) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	db.began = true
	return nil, errors.New("unexpected Google transaction")
}

func TestGoogleSession_RefusesReservedSubjectBeforeDatabaseAccess(t *testing.T) {
	db := &reservedGoogleDB{}
	_, err := store.New(db).CreateGoogleSession(context.Background(), store.GoogleSessionParams{Identity: store.GoogleIdentity{Sub: "loadcheck:reserved", Email: "synthetic@example.test"}})
	if !errors.Is(err, store.ErrLoginFailed) || db.began {
		t.Fatalf("reserved subject reached database: began=%v error=%v", db.began, err)
	}
}
