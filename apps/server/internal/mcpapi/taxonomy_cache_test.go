package mcpapi

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"danny.vn/trachuan/internal/retrieval"
)

type nodeCacheEngine struct {
	*cachedEngine
	err error
}

func (e *nodeCacheEngine) Taxonomy(context.Context, retrieval.TaxonomyRequest) (retrieval.TaxonomyResult, error) {
	return retrieval.TaxonomyResult{}, nil
}

func (e *nodeCacheEngine) TaxonomyNode(_ context.Context, code string) (retrieval.NodePage, error) {
	e.calls.Add(1)
	if e.err != nil {
		return retrieval.NodePage{}, e.err
	}
	return retrieval.NodePage{Release: e.release, Node: retrieval.NodeEntry{Code: code, Name: "Name"}, Path: []retrieval.NodeEntry{}, Documents: []retrieval.NodeDocument{}}, nil
}

type nodeCacheResolver struct{ engine *nodeCacheEngine }

func (r nodeCacheResolver) Resolve(context.Context, string, string) (Lease, error) {
	return Lease{Engine: r.engine, Release: r.engine.release, Generation: r.engine.generation, Done: func() {}}, nil
}

func TestTaxonomyNodeCache_ReleaseAndCode(t *testing.T) {
	e := &nodeCacheEngine{cachedEngine: &cachedEngine{release: "20990101", generation: 1}}
	c := NewCaches(time.Now)
	c.ActivateRelease(e.release, e.generation)
	o := NewOperations(Config{Resolver: nodeCacheResolver{e}, Caches: c})
	call := func(code string) []byte {
		t.Helper()
		out, err := o.TaxonomyNode(context.Background(), code)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	first := call("1")
	if second := call("1"); !bytes.Equal(first, second) || e.calls.Load() != 1 {
		t.Fatalf("hit differs or recomputed: %s / %s, calls %d", first, second, e.calls.Load())
	}
	if other := call("2"); bytes.Equal(first, other) || e.calls.Load() != 2 {
		t.Fatal("different code did not miss")
	}
	e.release, e.generation = "20990102", 2
	c.ActivateRelease(e.release, e.generation)
	if next := call("1"); bytes.Equal(first, next) || e.calls.Load() != 3 {
		t.Fatal("new release did not miss")
	}
	e.release, e.generation = "20990101", 3
	c.ActivateRelease(e.release, e.generation)
	if rollback := call("1"); !bytes.Equal(first, rollback) || e.calls.Load() != 4 {
		t.Fatal("rollback did not miss")
	}
}

func TestTaxonomyNodeCache_ErrorsAndNilCache(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{{"invalid", retrieval.ErrInvalidRequest}, {"not found", retrieval.ErrNotFound}, {"unavailable", retrieval.ErrUnavailable}} {
		t.Run(tc.name, func(t *testing.T) {
			e := &nodeCacheEngine{cachedEngine: &cachedEngine{release: "20990101", generation: 1}, err: tc.err}
			c := NewCaches(time.Now)
			c.ActivateRelease(e.release, e.generation)
			o := NewOperations(Config{Resolver: nodeCacheResolver{e}, Caches: c})
			for range 2 {
				if _, err := o.TaxonomyNode(context.Background(), "1"); !errors.Is(err, tc.err) {
					t.Fatalf("got %v, want %v", err, tc.err)
				}
			}
			if e.calls.Load() != 2 {
				t.Fatal("error was cached")
			}
			e.err = nil
			if _, err := o.TaxonomyNode(context.Background(), "1"); err != nil || e.calls.Load() != 3 {
				t.Fatalf("success after error: %v, calls %d", err, e.calls.Load())
			}
		})
	}
	t.Run("nil cache", func(t *testing.T) {
		e := &nodeCacheEngine{cachedEngine: &cachedEngine{release: "20990101", generation: 1}}
		o := NewOperations(Config{Resolver: nodeCacheResolver{e}})
		for range 2 {
			if _, err := o.TaxonomyNode(context.Background(), "1"); err != nil {
				t.Fatal(err)
			}
		}
		if e.calls.Load() != 2 {
			t.Fatal("nil cache did not compute each request")
		}
	})
}
