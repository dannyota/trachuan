// Package embedcache caches query vectors without retaining query text.
package embedcache

import (
	"context"
	"crypto/sha256"
	"math"
	"slices"
	"time"

	"danny.vn/trachuan/apps/server/internal/cache"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
)

// Client wraps an embedder pinned to one query recipe. Wrappers for different
// releases can share the vector cache, including across recipe changes.
type Client struct {
	next    retrieval.Embedder
	hash    string
	recipe  embedding.Recipe
	vectors *cache.LRU[embedding.Vector]
}

// NewVectors creates the 256 MB, 24-hour query-vector cache.
func NewVectors(clock func() time.Time) *cache.LRU[embedding.Vector] {
	return cache.New("query_vectors", 256_000_000, 24*time.Hour, clock, func(v embedding.Vector) int { return len(v.Values)*4 + len(v.RecipeHash) + 8 })
}

// New wraps next with the verified query recipe and shared vectors.
// A nil vectors cache disables caching.
func New(next retrieval.Embedder, recipe embedding.Recipe, vectors *cache.LRU[embedding.Vector]) *Client {
	return &Client{next: next, hash: recipe.Hash(), recipe: recipe, vectors: vectors}
}

// Recipe forwards the embedder's recipe and its cancellation unchanged.
func (c *Client) Recipe(ctx context.Context) (embedding.Recipe, error) { return c.next.Recipe(ctx) }

// Embed computes every miss independently under the caller's context.
func (c *Client) Embed(ctx context.Context, text string) (embedding.Vector, error) {
	if c.vectors == nil {
		return c.next.Embed(ctx, text)
	}
	if err := ctx.Err(); err != nil {
		return embedding.Vector{}, err
	}
	h := sha256.New()
	_, _ = h.Write([]byte(c.hash))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(text))
	key := [32]byte(h.Sum(nil))
	if v, ok := c.vectors.Get(key); ok {
		v.Values = slices.Clone(v.Values)
		return v, nil
	}
	v, err := c.next.Embed(ctx, text)
	if err == nil && ctx.Err() == nil && valid(v, c.hash, c.recipe) {
		stored := v
		stored.Values = slices.Clone(v.Values)
		c.vectors.Put(key, stored)
	}
	return v, err
}

func valid(v embedding.Vector, hash string, recipe embedding.Recipe) bool {
	if v.RecipeHash != hash || v.TokenCount <= 0 || v.TokenCount > recipe.TokenCap || len(v.Values) != recipe.Dimensions || len(v.Values) == 0 {
		return false
	}
	var norm float64
	for _, x := range v.Values {
		norm += float64(x) * float64(x)
	}
	return !math.IsNaN(norm) && !math.IsInf(norm, 0) && math.Abs(math.Sqrt(norm)-1) <= .001
}
