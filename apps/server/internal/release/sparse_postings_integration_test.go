//go:build integration

package release

import (
	"context"
	"errors"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

func TestResolver_PostingsOutliveRetirementUntilLastLease(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		t.Run(map[bool]string{false: "activation", true: "shutdown"}[shutdown], func(t *testing.T) {
			r, _, _, rel := corpusFixture(t)
			outcomes := make(chan retrieval.PostingsOutcome, 1)
			r.OnPostings = func(_ string, outcome retrieval.PostingsOutcome) { outcomes <- outcome }
			lease := takeLease(t, r, rel)
			if result := <-outcomes; result.Status != "ready" || !lease.Engine.SparsePostingsReady() {
				t.Fatalf("postings outcome: %+v", result)
			}
			entry := r.entries["corpus_vn_"+rel]
			if shutdown {
				r.Close()
			} else {
				store := r.Auth.(*fakeStore)
				store.mu.Lock()
				store.active = ""
				store.mu.Unlock()
				if _, err := r.Resolve(t.Context(), "vn", rel); !errors.Is(err, ErrExpired) {
					t.Fatal(err)
				}
			}
			if !lease.Engine.SparsePostingsReady() {
				t.Fatal("retirement discarded published postings")
			}
			lease.Done()
			if lease.Engine.SparsePostingsReady() {
				t.Fatal("last lease did not close engine")
			}
			if conn, err := entry.pool.Acquire(t.Context()); err == nil {
				conn.Release()
				t.Fatal("last lease did not close pool")
			}
		})
	}
}

func TestResolver_OpenFailureClosesEngineBeforePool(t *testing.T) {
	r, _, db, rel := corpusFixture(t)
	metadata, err := retrieval.ReadRelease(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	e, err := retrieval.Open(t.Context(), db, recipeEmbedder{recipe: metadata.DenseRecipe.Query}, retrieval.Options{Now: r.Clock, Profile: "serving"})
	if err != nil {
		t.Fatal(err)
	}
	outcomes := make(chan retrieval.PostingsOutcome, 1)
	e.StartSparsePostings(func(result retrieval.PostingsOutcome) { outcomes <- result })
	if result := <-outcomes; result.Status != "ready" {
		t.Fatal(result)
	}
	closed := false
	r.Open = func(context.Context, string) (*retrieval.Engine, func(), error) {
		return e, func() {
			if e.SparsePostingsReady() {
				t.Error("pool closed before engine")
			}
			closed = true
		}, errors.New("open failed after engine creation")
	}
	if _, err = r.Resolve(t.Context(), "vn", rel); err == nil || !closed {
		t.Fatalf("error=%v closed=%t", err, closed)
	}
	if e.SparsePostingsReady() {
		t.Fatal("failed open retained engine")
	}
}
