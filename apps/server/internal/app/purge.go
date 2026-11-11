package app

import (
	"context"
	"log/slog"
	"time"

	"danny.vn/trachuan/apps/server/internal/accountapi"
)

type purgeStore interface {
	accountapi.PurgeStore
	DeleteExpiredLoginTransactions(context.Context) error
}

func purgeLoop(ctx context.Context, store purgeStore, clock func() time.Time, logger *slog.Logger) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	runPurgeLoop(ctx, store, clock, logger, ticker.C)
}

func runPurgeLoop(ctx context.Context, store purgeStore, clock func() time.Time, logger *slog.Logger, ticks <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			queryCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			err := store.DeleteExpiredLoginTransactions(queryCtx)
			cancel()
			if err != nil {
				logger.Error("purge failed", "step", "login_transactions")
			}
			if _, err := accountapi.Purge(ctx, store, clock()); err != nil {
				logger.Error("purge failed", "step", "deleted_accounts")
			}
		}
	}
}
