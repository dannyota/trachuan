package retrieval

import (
	"cmp"
	"context"
	"errors"
	"math/rand/v2"
	"slices"
	"testing"
)

type sparseCancelContext struct {
	context.Context
	remaining int
	cancel    context.CancelFunc
}

func (c *sparseCancelContext) Err() error {
	c.remaining--
	if c.remaining <= 0 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestSparseSort_OrdersDimensionsAndChunkPositions(t *testing.T) {
	rng := rand.New(rand.NewPCG(73, 81))
	for _, length := range []int{0, 1, 2, 100, 10000} {
		triples := make([]sparseTriple, length)
		for i := range triples {
			triples[i] = sparseTriple{dimension: int32(rng.IntN(8)), position: uint32(i), weight: float32(i)}
		}
		want := slices.Clone(triples)
		order := func(a, b sparseTriple) int {
			if n := cmp.Compare(a.dimension, b.dimension); n != 0 {
				return n
			}
			return cmp.Compare(a.position, b.position)
		}
		slices.SortFunc(want, order)
		if err := sortSparseTriples(t.Context(), triples); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(triples, want) {
			t.Fatal("sort differs from reference")
		}
	}
}

func TestSparseBuild_CancelsDuringCPUWork(t *testing.T) {
	for _, sort := range []bool{false, true} {
		triples := make([]sparseTriple, 100000)
		for i := range triples {
			triples[i] = sparseTriple{dimension: int32(len(triples) - i), position: uint32(i), weight: 1}
		}
		parent, cancel := context.WithCancel(t.Context())
		defer cancel()
		ctx := &sparseCancelContext{Context: parent, remaining: 4, cancel: cancel}
		var err error
		if sort {
			err = sortSparseTriples(ctx, triples)
		} else {
			err = (&sparsePostings{}).buildDirectory(ctx, triples)
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("sort=%t error=%v", sort, err)
		}
	}
}

func TestSparseBuild_DirectoryPreservesPositionsAndWeights(t *testing.T) {
	index := &sparsePostings{}
	triples := []sparseTriple{{dimension: 1, position: 0, weight: 1}, {dimension: 1, position: 3, weight: 2}, {dimension: 5, position: 2, weight: 4}}
	if err := index.buildDirectory(t.Context(), triples); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(index.dimensions, []sparseDimension{{index: 1, start: 0, end: 2}, {index: 5, start: 2, end: 3}}) {
		t.Fatalf("dimensions=%v", index.dimensions)
	}
	if !slices.Equal(index.entries, []sparsePosting{{position: 0, weight: 1}, {position: 3, weight: 2}, {position: 2, weight: 4}}) {
		t.Fatalf("entries=%v", index.entries)
	}
}
