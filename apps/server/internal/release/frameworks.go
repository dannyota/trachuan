package release

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/frameworks/serve"
	"github.com/jackc/pgx/v5/pgxpool"
)

func (r *Resolver) prefix(country string) string {
	if r.Frameworks != nil {
		return "frameworks_"
	}
	return "corpus_" + country + "_"
}

func (r *Resolver) validName(country, name string) bool {
	if r.Frameworks != nil {
		return regexp.MustCompile(`^frameworks_[0-9]{8}(_[0-9]{1,2})?$`).MatchString(name)
	}
	return validDBName(country, name)
}

func (r *Resolver) openFrameworks(ctx context.Context, name string, entry *cacheEntry) error {
	if r.OpenFrameworks != nil {
		engine, closePool, err := r.OpenFrameworks(ctx, name)
		r.mu.Lock()
		entry.frameworks, entry.closePool = engine, closePool
		r.mu.Unlock()
		return err
	}
	if r.EmbedderFor == nil {
		return errors.New("missing frameworks embedder factory")
	}
	cfg, err := poolConfig(r.URLTemplate, name)
	if err != nil {
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return serve.ErrUnavailable
	}
	r.mu.Lock()
	if r.closed || r.entries[name] != entry {
		r.mu.Unlock()
		pool.Close()
		return ErrExpired
	}
	entry.pool, entry.closePool = pool, pool.Close
	r.mu.Unlock()
	query := embedding.QueryRecipe1()
	engine, err := serve.Open(ctx, pool, r.EmbedderFor(query.Hash(), query), *r.Frameworks)
	if err != nil {
		return err
	}
	if engine.Release() != strings.TrimPrefix(name, "frameworks_") {
		return serve.ErrUnavailable
	}
	r.mu.Lock()
	entry.frameworks = engine
	r.mu.Unlock()
	return nil
}
