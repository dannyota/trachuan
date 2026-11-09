package build

import (
	"context"
	"fmt"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/internal/retrieval"
	"github.com/jackc/pgx/v5/pgxpool"
)

func calibrate(ctx context.Context, cfg Config, db *pgxpool.Pool, release retrieval.Release) (evaluation.Calibration, error) {
	if cfg.Calibrate != nil {
		return cfg.Calibrate(ctx, db, release)
	}
	date, err := time.Parse("20060102", release.Rel[:8])
	if err != nil || date.Year() == 0 {
		return evaluation.Calibration{}, fmt.Errorf("invalid release date")
	}
	engine, err := retrieval.OpenDraft(ctx, db, cfg.Embedder, release, retrieval.Options{Profile: "evaluation", Now: func() time.Time { return date }})
	if err != nil {
		return evaluation.Calibration{}, fmt.Errorf("open draft corpus: %w", err)
	}
	result, err := evaluation.Calibrate(ctx, engine, cfg.Suite)
	if err != nil {
		return evaluation.Calibration{}, fmt.Errorf("calibrate corpus: %w", err)
	}
	return result, nil
}
