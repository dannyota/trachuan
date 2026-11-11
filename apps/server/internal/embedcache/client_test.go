package embedcache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"danny.vn/trachuan/internal/embedding"
)

type embedder struct {
	invalid bool
	calls   atomic.Int64
	recipe  embedding.Recipe
	fail    bool
	entered chan struct{}
	unblock chan struct{}
}

func (e *embedder) Recipe(context.Context) (embedding.Recipe, error) { return e.recipe, nil }
func (e *embedder) Embed(ctx context.Context, _ string) (embedding.Vector, error) {
	e.calls.Add(1)
	if e.entered != nil {
		e.entered <- struct{}{}
		select {
		case <-e.unblock:
		case <-ctx.Done():
			return embedding.Vector{}, ctx.Err()
		}
	}
	if e.fail {
		return embedding.Vector{}, errors.New("failed")
	}
	values := make([]float32, e.recipe.Dimensions)
	values[0] = 1
	if e.invalid {
		values = values[:1]
	}
	return embedding.Vector{Values: values, TokenCount: 1, RecipeHash: e.recipe.Hash()}, nil
}
func TestQueryVectorCache_KeyIsRecipeAndText(t *testing.T) {
	vectors := NewVectors(time.Now)
	e := &embedder{recipe: embedding.QueryRecipe1()}
	a := New(e, e.recipe, vectors)
	ctx := context.Background()
	v, err := a.Embed(ctx, "exact text")
	if err != nil {
		t.Fatal(err)
	}
	v.Values[0] = 9
	v, err = a.Embed(ctx, "exact text")
	if err != nil || v.Values[0] != 1 || e.calls.Load() != 1 {
		t.Fatal("hit changed or missed")
	}
	b := New(e, e.recipe, vectors)
	_, _ = b.Embed(ctx, "exact text")
	if e.calls.Load() != 1 {
		t.Fatal("same recipe across wrappers missed")
	}
	_, _ = a.Embed(ctx, "exact text ")
	e.recipe.Precision = "changed"
	b = New(e, e.recipe, vectors)
	_, _ = b.Embed(ctx, "exact text")
	if e.calls.Load() != 3 {
		t.Fatal("text or recipe not in key")
	}
}
func TestCache_ErrorsNotCached(t *testing.T) {
	e := &embedder{recipe: embedding.QueryRecipe1(), fail: true}
	c := New(e, e.recipe, NewVectors(time.Now))
	for i := 0; i < 2; i++ {
		if _, err := c.Embed(context.Background(), "text"); err == nil {
			t.Fatal("missing error")
		}
	}
	if e.calls.Load() != 2 {
		t.Fatal("error cached")
	}
	e.fail = false
	c.hash = "wrong"
	for i := 0; i < 2; i++ {
		_, _ = c.Embed(context.Background(), "text")
	}
	if e.calls.Load() != 4 {
		t.Fatal("wrong-recipe vector cached")
	}
}
func TestCache_ConcurrentMissesComputeIndependently(t *testing.T) {
	e := &embedder{recipe: embedding.QueryRecipe1(), entered: make(chan struct{}, 2), unblock: make(chan struct{})}
	c := New(e, e.recipe, NewVectors(time.Now))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, b := make(chan error, 1), make(chan error, 1)
	var wg sync.WaitGroup
	wg.Go(func() { _, err := c.Embed(ctx, "text"); a <- err })
	wg.Go(func() { _, err := c.Embed(context.Background(), "text"); b <- err })
	defer close(e.unblock)
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	for i := 0; i < 2; i++ {
		select {
		case <-e.entered:
		case <-timer.C:
			t.Fatal("misses shared work")
		}
	}
	cancel()
	if err := <-a; !errors.Is(err, context.Canceled) {
		t.Fatal("caller did not cancel", err)
	}
	e.unblock <- struct{}{}
	if err := <-b; err != nil {
		t.Fatal("one caller canceled another", err)
	}
	wg.Wait()
	if e.calls.Load() != 2 {
		t.Fatal("misses shared work")
	}
}

func TestQueryVectorCache_InvalidInferenceNotRetained(t *testing.T) {
	e := &embedder{recipe: embedding.QueryRecipe1(), invalid: true}
	c := New(e, e.recipe, NewVectors(time.Now))
	// This fake returns a unit vector with the wrong dimension. Retrieval would
	// reject it, so the wrapper must not turn that inference failure into a hit.
	for i := 0; i < 2; i++ {
		_, _ = c.Embed(context.Background(), "text")
	}
	if e.calls.Load() != 2 {
		t.Fatal("invalid inference retained")
	}
}

func TestQueryVectorCache_CanceledHit(t *testing.T) {
	e := &embedder{recipe: embedding.QueryRecipe1()}
	c := New(e, e.recipe, NewVectors(time.Now))
	_, _ = c.Embed(context.Background(), "text")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Embed(ctx, "text"); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled hit succeeded")
	}
	if e.calls.Load() != 1 {
		t.Fatal("canceled hit reached embedder")
	}
}
