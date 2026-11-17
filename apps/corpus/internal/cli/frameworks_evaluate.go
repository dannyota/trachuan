package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"

	lawbuild "danny.vn/trachuan/apps/corpus/internal/stage/build"
	"danny.vn/trachuan/internal/frameworks/build"
	"danny.vn/trachuan/internal/frameworks/evaluate"
	"danny.vn/trachuan/internal/frameworks/serve"
	"danny.vn/trachuan/internal/retrieval"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func (d *frameworkDatabase) Evaluate(ctx context.Context, req build.EvaluationRequest, publish func(build.EvaluationResult) error) error {
	query, err := newFrameworkQueryEmbedder()
	if err != nil {
		return err
	}
	cache := evaluate.NewQueryCache(query)
	if err = cache.Prepare(ctx, req.Cases); err != nil {
		return err
	}
	open := func(ctx context.Context, conn *pgx.Conn, key []byte) (evaluate.Engine, func(), error) {
		return openFrameworkEvaluation(ctx, conn, key, cache)
	}
	return evaluate.Restored(ctx, req, d.restore, open, publish)
}

func (d *frameworkDatabase) restore(ctx context.Context, req build.DatabaseRequest, suffix string, temporary bool, use func(context.Context, *pgx.Conn) error) error {
	if !regexp.MustCompile(`^[0-9]{8}(_[0-9]{1,2})?$`).MatchString(req.Rel) || req.WorkDir == "" || req.RunID == "" || !regexp.MustCompile(`^_eval(_mut_[0-9]+)?$`).MatchString(suffix) || temporary != (suffix != "_eval") {
		return fmt.Errorf("invalid evaluation database identity")
	}
	if err := localFrameworkDatabase(d.cfg.BuilderURL); err != nil {
		return err
	}
	cfg := d.cfg
	cfg.PgRestore = os.Getenv("CORPUS_PG_RESTORE")
	if cfg.PgRestore == "" {
		cfg.PgRestore = "pg_restore"
	}
	marker := frameworkDatabaseMarker{req.WorkDir, req.Rel, req.RunID}
	raw, err := json.Marshal(marker)
	if err != nil {
		return fmt.Errorf("encode evaluation marker: %w", err)
	}
	name := "frameworks_" + req.Rel + suffix
	withDatabase := lawbuild.WithDatabase
	if temporary {
		withDatabase = lawbuild.WithTemporaryDatabase
	}
	err = withDatabase(ctx, cfg, name, string(raw), marker.owns, func(ctx context.Context, conn *pgx.Conn) error {
		if restoreErr := lawbuild.RestoreDatabase(ctx, cfg, name, req.DumpPath); restoreErr != nil {
			return fmt.Errorf("restore evaluation copy: %w", &frameworkEmbedError{cause: restoreErr})
		}
		return use(ctx, conn)
	})
	if err != nil {
		return fmt.Errorf("evaluation database: %w", &frameworkEmbedError{cause: err})
	}
	return nil
}

func openFrameworkEvaluation(ctx context.Context, conn *pgx.Conn, key []byte, query retrieval.Embedder) (evaluate.Engine, func(), error) {
	config, err := pgxpool.ParseConfig(conn.Config().ConnString())
	if err != nil {
		return nil, nil, fmt.Errorf("configure evaluation database: %w", &frameworkEmbedError{cause: err})
	}
	config.ConnConfig = conn.Config().Copy()
	config.MaxConns = 1
	config.ConnConfig.Tracer = evaluate.CauseTracer{}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, nil, fmt.Errorf("open evaluation database: %w", &frameworkEmbedError{cause: err})
	}
	engine, err := evaluate.OpenWithFailureCauses(ctx, func(ctx context.Context) (evaluate.Engine, error) {
		return serve.Open(ctx, pool, query, serve.Options{CursorKey: key})
	})
	if err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("open evaluation engine: %w", &frameworkEmbedError{cause: err})
	}
	return evaluate.WithFailureCauses(engine), pool.Close, nil
}
