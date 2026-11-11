package oauth

import (
	"context"
	"log/slog"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
)

// RunRetention removes expired OAuth records until the context is canceled.
func RunRetention(ctx context.Context, queries *store.Queries, logger *slog.Logger) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			retentionPass(ctx, queries, time.Now(), logger)
		}
	}
}

func retentionPass(ctx context.Context, queries *store.Queries, now time.Time, logger *slog.Logger) {
	const batchLimit = 1000
	day := pgtype.Interval{Microseconds: int64(24 * time.Hour / time.Microsecond), Valid: true}
	month := pgtype.Interval{Microseconds: int64(30 * 24 * time.Hour / time.Microsecond), Valid: true}
	steps := []struct {
		name string
		run  func(context.Context) (int64, error)
	}{
		{"requests", func(ctx context.Context) (int64, error) {
			return queries.DeleteExpiredOAuthRequests(ctx, store.DeleteExpiredOAuthRequestsParams{AsOf: now, Retention: day, BatchLimit: batchLimit})
		}},
		{"codes", func(ctx context.Context) (int64, error) {
			return queries.DeleteExpiredOAuthCodes(ctx, store.DeleteExpiredOAuthCodesParams{AsOf: now, Retention: day, BatchLimit: batchLimit})
		}},
		{"tokens", func(ctx context.Context) (int64, error) {
			return queries.DeleteExpiredOAuthTokens(ctx, store.DeleteExpiredOAuthTokensParams{AsOf: now, Retention: day, BatchLimit: batchLimit})
		}},
		{"grants", func(ctx context.Context) (int64, error) {
			return queries.DeleteRetiredOAuthGrants(ctx, store.DeleteRetiredOAuthGrantsParams{AsOf: now, Retention: month, BatchLimit: batchLimit})
		}},
	}
	for _, step := range steps {
		for range 50 {
			if ctx.Err() != nil {
				return
			}
			queryCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			deleted, err := step.run(queryCtx)
			cancel()
			if err != nil {
				if ctx.Err() == nil {
					logger.Error("oauth retention failed", "step", step.name)
				}
				break
			}
			if deleted < batchLimit {
				break
			}
		}
	}
}
