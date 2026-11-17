package app

import (
	"context"
	"errors"
	"time"

	"danny.vn/trachuan/apps/server/internal/mcpapi"
	"danny.vn/trachuan/apps/server/internal/release"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/internal/embedding"
	frameworkserve "danny.vn/trachuan/internal/frameworks/serve"
	"danny.vn/trachuan/internal/retrieval"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type frameworksStore struct{ db *pgxpool.Pool }

func (s frameworksStore) GetActiveRelease(ctx context.Context, country string) (string, error) {
	var name string
	err := s.db.QueryRow(ctx, `SELECT active_db FROM frameworks_release WHERE country=$1`, country).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return name, err
}

type frameworksResolver struct{ resolver *release.Resolver }

func (r frameworksResolver) ResolveFrameworks(ctx context.Context, rel string) (mcpapi.FrameworkLease, error) {
	lease, err := r.resolver.Resolve(ctx, "vn", rel)
	if err != nil {
		if errors.Is(err, release.ErrExpired) && rel != "" {
			return mcpapi.FrameworkLease{}, frameworkserve.ErrReleaseExpired
		}
		return mcpapi.FrameworkLease{}, frameworkserve.ErrUnavailable
	}
	return mcpapi.FrameworkLease{Engine: lease.Frameworks, Done: lease.Done}, nil
}

func configureFrameworks(cfg config, pool *pgxpool.Pool, queries *store.Queries, client *embedding.Client, tools *mcpapi.Config) func() {
	if !cfg.frameworks {
		return func() {}
	}
	resolver := &release.Resolver{Auth: frameworksStore{pool}, URLTemplate: cfg.corpusURL, Clock: time.Now,
		Frameworks: &frameworkserve.Options{CursorKey: cfg.frameworksKey},
		EmbedderFor: func(hash string, expected embedding.Recipe) retrieval.Embedder {
			return embedding.Query{Client: client, RecipeHash: hash, ExpectedRecipe: &expected}
		}}
	tools.Frameworks = true
	tools.FrameworkResolver = frameworksResolver{resolver}
	tools.FrameworkAccess = frameworkCaller(queries)
	return resolver.Close
}
