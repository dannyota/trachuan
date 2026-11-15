package app

import (
	"context"
	"errors"
	"io"
	"testing"

	"danny.vn/trachuan/apps/server/internal/store"
)

type restoreFailureStore struct {
	adminStore
	err error
}

func (s restoreFailureStore) RestoreFence(context.Context) error { return s.err }

func TestAdmin_RestoreFenceReportsSchemaError(t *testing.T) {
	for _, tc := range []struct {
		name    string
		failure error
		want    string
	}{
		{name: "unmigrated", failure: store.ErrAuthSchemaNotMigrated, want: "auth schema is not migrated; run migrate first"},
		{name: "store failure", failure: errors.New("synthetic private database failure"), want: "restore fence store unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := runAdmin(context.Background(), []string{"restore-fence"}, restoreFailureStore{err: tc.failure}, io.Discard)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error=%v want=%q", err, tc.want)
			}
		})
	}
}
