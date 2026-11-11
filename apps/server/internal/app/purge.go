package app

import (
	"context"
	"time"

	"danny.vn/trachuan/apps/server/internal/accountapi"
)

type purgeStore interface {
	accountapi.PurgeStore
	DeleteExpiredLoginTransactions(context.Context) error
}

func purgeLoop(ctx context.Context, store purgeStore, clock func() time.Time) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			queryCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			_ = store.DeleteExpiredLoginTransactions(queryCtx)
			cancel()
			_, _ = accountapi.Purge(ctx, store, clock())
		}
	}
}
