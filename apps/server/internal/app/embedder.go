package app

import (
	"context"
	"errors"
	"sync"

	"danny.vn/trachuan/apps/server/internal/cache"
	"danny.vn/trachuan/apps/server/internal/embedcache"
	"danny.vn/trachuan/internal/embedding"
)

// cachedQuery installs the cache only after the service confirms the release hash.
type cachedQuery struct {
	query   embedding.Query
	vectors *cache.LRU[embedding.Vector]
	mu      sync.RWMutex
	cached  *embedcache.Client
}

func (q *cachedQuery) Recipe(ctx context.Context) (embedding.Recipe, error) {
	recipe, err := q.query.Recipe(ctx)
	if err != nil {
		return embedding.Recipe{}, err
	}
	q.mu.Lock()
	q.cached = embedcache.New(q.query, recipe, q.vectors)
	q.mu.Unlock()
	return recipe, nil
}
func (q *cachedQuery) Embed(ctx context.Context, text string) (embedding.Vector, error) {
	q.mu.RLock()
	client := q.cached
	q.mu.RUnlock()
	if client == nil {
		return embedding.Vector{}, errors.New("query recipe not verified")
	}
	return client.Embed(ctx, text)
}
