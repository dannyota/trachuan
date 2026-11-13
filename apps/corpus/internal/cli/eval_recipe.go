package cli

import (
	"context"
	"fmt"
	"os"

	"danny.vn/trachuan/apps/corpus/internal/config"
	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"github.com/jackc/pgx/v5/pgxpool"
)

// readEvalRecipeVersion reads only release metadata, before preparing an evaluation.
func readEvalRecipeVersion(ctx context.Context, cc, rel string) (int, error) {
	name, err := build.DatabaseName(cc, rel)
	if err != nil {
		return 0, err
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return 0, err
	}
	if cfg.BuilderDatabaseURL == "" {
		return 0, fmt.Errorf("CORPUS_BUILDER_DATABASE_URL is required")
	}
	poolConfig, err := pgxpool.ParseConfig(cfg.BuilderDatabaseURL)
	if err != nil {
		return 0, fmt.Errorf("invalid evaluation database configuration")
	}
	poolConfig.ConnConfig.Database = name
	db, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return 0, fmt.Errorf("open release metadata: %w", err)
	}
	defer db.Close()
	var version int
	if err = db.QueryRow(ctx, `SELECT (retrieval_recipe->>'version')::int FROM release WHERE singleton`).Scan(&version); err != nil {
		return 0, fmt.Errorf("read release retrieval recipe version failed")
	}
	return version, nil
}
