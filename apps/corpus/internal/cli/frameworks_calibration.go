package cli

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"

	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/frameworks/build"
	"danny.vn/trachuan/internal/frameworks/serve"
	"danny.vn/trachuan/internal/retrieval"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newFrameworkQueryEmbedder() (retrieval.Embedder, error) {
	address := os.Getenv("CORPUS_QUERY_EMBEDDER_URL")
	if address == "" {
		address = "http://127.0.0.1:8091"
	}
	client, err := localFrameworkEmbedderClient(address)
	if err != nil {
		return nil, err
	}
	r := embedding.QueryRecipe1()
	return embedding.Query{Client: client, RecipeHash: r.Hash(), ExpectedRecipe: &r}, nil
}

func openFrameworkCalibration(ctx context.Context, conn *pgx.Conn) (build.CalibrationEngine, func(), error) {
	query, err := newFrameworkQueryEmbedder()
	if err != nil {
		return nil, nil, err
	}
	config, err := pgxpool.ParseConfig(conn.Config().ConnString())
	if err != nil {
		return nil, nil, fmt.Errorf("configure calibration database: %w", &frameworkEmbedError{cause: err})
	}
	config.ConnConfig = conn.Config().Copy()
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, nil, fmt.Errorf("open calibration database: %w", &frameworkEmbedError{cause: err})
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("create calibration cursor key: %w", &frameworkEmbedError{cause: err})
	}
	engine, err := serve.Open(ctx, pool, query, serve.Options{CursorKey: key})
	if err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("open calibration engine: %w", &frameworkEmbedError{cause: err})
	}
	return engine, pool.Close, nil
}
