package app

import (
	"context"
	"errors"

	"danny.vn/trachuan/apps/server/internal/store"
)

type restoreFenceStore interface{ RestoreFence(context.Context) error }

func runRestoreFence(ctx context.Context, args []string, queries adminStore) error {
	if len(args) != 0 {
		return errors.New("invalid restore-fence arguments")
	}
	q, ok := queries.(restoreFenceStore)
	if !ok {
		return errors.New("restore fence store unavailable")
	}
	if err := q.RestoreFence(ctx); err != nil {
		if errors.Is(err, store.ErrAuthSchemaNotMigrated) {
			return store.ErrAuthSchemaNotMigrated
		}
		return errors.New("restore fence store unavailable")
	}
	return nil
}
