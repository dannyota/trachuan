package evaluate

import (
	"context"
	"slices"
	"sync"

	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
)

type cachedQuery struct {
	vector embedding.Vector
	err    error
}
type QueryCache struct {
	source       retrieval.Embedder
	mu           sync.Mutex
	queries      map[string]cachedQuery
	recipe       embedding.Recipe
	recipeErr    error
	recipeLoaded bool
}

func NewQueryCache(source retrieval.Embedder) *QueryCache {
	return &QueryCache{source: source, queries: map[string]cachedQuery{}}
}
func (c *QueryCache) Recipe(ctx context.Context) (embedding.Recipe, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.recipeLoaded {
		c.recipe, c.recipeErr = c.source.Recipe(ctx)
		c.recipeLoaded = true
	}
	recordCause(ctx, c.recipeErr)
	return c.recipe, c.recipeErr
}
func (c *QueryCache) Embed(ctx context.Context, text string) (embedding.Vector, error) {
	if err := ctx.Err(); err != nil {
		recordCause(ctx, err)
		return embedding.Vector{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	value, ok := c.queries[text]
	if !ok {
		value.vector, value.err = c.source.Embed(ctx, text)
		value.vector.Values = slices.Clone(value.vector.Values)
		c.queries[text] = value
	}
	value.vector.Values = slices.Clone(value.vector.Values)
	recordCause(ctx, value.err)
	return value.vector, value.err
}
func (c *QueryCache) Prepare(ctx context.Context, cases []GoldenCase) error {
	_, _ = c.Recipe(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, item := range cases {
		_, _ = c.Embed(ctx, item.Question)
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return nil
}
