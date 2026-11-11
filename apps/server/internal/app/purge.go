package app

import (
	"context"
	"log/slog"
	"time"

	"danny.vn/trachuan/apps/server/internal/accountapi"
	"danny.vn/trachuan/apps/server/internal/store"
)

type purgeStore interface {
	accountapi.PurgeStore
	DeleteExpiredLoginTransactions(context.Context) error
	PurgeEndedCredentials(context.Context) (int64, error)
	DeleteExpiredReports(context.Context, store.DeleteExpiredReportsParams) (int64, error)
	DeleteExpiredSnapshots(context.Context, store.DeleteExpiredSnapshotsParams) (int64, error)
}

func purgeLoop(ctx context.Context, s purgeStore, clock func() time.Time, logger *slog.Logger) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	runPurgeLoop(ctx, s, clock, logger, ticker.C)
}

func runPurgeLoop(ctx context.Context, s purgeStore, clock func() time.Time, logger *slog.Logger, ticks <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			queryCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			err := s.DeleteExpiredLoginTransactions(queryCtx)
			cancel()
			if err != nil {
				logger.Error("purge failed", "step", "login_transactions")
			}
			queryCtx, cancel = context.WithTimeout(ctx, 2*time.Second)
			_, err = s.PurgeEndedCredentials(queryCtx)
			cancel()
			if err != nil {
				logger.Error("purge failed", "step", "ended_credentials")
			}
			before := clock().Add(-90 * 24 * time.Hour)
			purgeReportBatches(ctx, logger, "reports", func(ctx context.Context) (int64, error) {
				return s.DeleteExpiredReports(ctx, store.DeleteExpiredReportsParams{Before: before, BatchLimit: 1000})
			})
			purgeReportBatches(ctx, logger, "report_snapshots", func(ctx context.Context) (int64, error) {
				return s.DeleteExpiredSnapshots(ctx, store.DeleteExpiredSnapshotsParams{Before: before, BatchLimit: 1000})
			})
			if _, err := accountapi.Purge(ctx, s, clock()); err != nil {
				logger.Error("purge failed", "step", "deleted_accounts")
			}
		}
	}
}

func purgeReportBatches(ctx context.Context, logger *slog.Logger, step string, run func(context.Context) (int64, error)) {
	for range 50 {
		if ctx.Err() != nil {
			return
		}
		queryCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		deleted, err := run(queryCtx)
		cancel()
		if err != nil {
			if ctx.Err() == nil {
				logger.Error("purge failed", "step", step)
			}
			return
		}
		if deleted < 1000 {
			return
		}
	}
}
