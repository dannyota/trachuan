package cli

import (
	"context"
	"fmt"
	"os"

	"danny.vn/trachuan/apps/corpus/internal/classify"
	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
	"github.com/jackc/pgx/v5/pgxpool"
)

type classificationEvidence struct {
	taxonomyHash string
	articles     []classify.Article
	nodes        []classify.Node
}

func loadClassificationEvidence(ctx context.Context, cc, rel string, labels taxonomy.Labels) (classificationEvidence, error) {
	var e classificationEvidence
	if labels.Country != cc {
		return e, fmt.Errorf("labeled set country differs from target release")
	}
	name, err := build.DatabaseName(cc, rel)
	if err != nil {
		return e, err
	}
	url := os.Getenv("CORPUS_BUILDER_DATABASE_URL")
	if url == "" {
		return e, fmt.Errorf("CORPUS_BUILDER_DATABASE_URL is required")
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return e, fmt.Errorf("invalid classification release database configuration")
	}
	cfg.ConnConfig.Database = name
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return e, fmt.Errorf("open classification release database: %w", err)
	}
	defer db.Close()
	if err = db.QueryRow(ctx, `SELECT taxonomy_version FROM release WHERE singleton AND country=$1 AND rel=$2`, cc, rel).Scan(&e.taxonomyHash); err != nil {
		return e, fmt.Errorf("read classification release identity: %w", err)
	}
	if e.nodes, err = classify.LoadNodes(ctx, db); err != nil {
		return e, err
	}
	e.articles, err = classify.LoadArticles(ctx, db, labels.Articles)
	return e, err
}
