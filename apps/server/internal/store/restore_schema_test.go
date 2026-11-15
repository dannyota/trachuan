package store_test

import (
	"context"
	"errors"
	"testing"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type restoreSchemaDB struct {
	store.DBTX
	version int64
	readErr error
	begun   bool
}

func (db *restoreSchemaDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return authorityRow{value: db.version, err: db.readErr}
}
func (db *restoreSchemaDB) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	db.begun = true
	return nil, errors.New("synthetic begin failure")
}

func TestRestoreFence_RejectsUnmigratedSchemaBeforeTransaction(t *testing.T) {
	for _, tc := range []struct {
		name    string
		version int64
		err     error
	}{
		{name: "previous", version: 8},
		{name: "empty", version: 0},
		{name: "ahead", version: 999999},
		{name: "missing version table", err: &pgconn.PgError{Code: "42P01"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := &restoreSchemaDB{version: tc.version, readErr: tc.err}
			err := store.New(db).RestoreFence(context.Background())
			if err == nil || err.Error() != "auth schema is not migrated; run migrate first" || db.begun {
				t.Fatalf("error=%v began=%v", err, db.begun)
			}
		})
	}
}
