//go:build integration

package release

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

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
			waitResolverCleanup(r)
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
	_, err = r.Resolve(t.Context(), "vn", rel)
	waitResolverCleanup(r)
	if err == nil || !closed {
		t.Fatalf("error=%v closed=%t", err, closed)
	}
	if e.SparsePostingsReady() {
		t.Fatal("failed open retained engine")
	}
}

func TestResolver_BlockedLoaderDoesNotDelayRetirementRequests(t *testing.T) {
	for _, lastLease := range []bool{false, true} {
		t.Run(map[bool]string{false: "activation", true: "last lease"}[lastLease], func(t *testing.T) {
			r, _, _, rel := corpusFixture(t)
			entered, release, returned, poolClosed := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
			var once sync.Once
			stop := func() { once.Do(func() { close(release) }) }
			defer r.Close()
			defer stop()
			r.OnPostings = func(_ string, outcome retrieval.PostingsOutcome) {
				if outcome.Status != "ready" {
					t.Errorf("postings outcome: %+v", outcome)
				}
				close(entered)
				<-release
				close(returned)
			}
			lease := takeLease(t, r, rel)
			<-entered
			r.mu.Lock()
			entry := r.entries["corpus_vn_"+rel]
			closePool := entry.closePool
			entry.closePool = func() {
				select {
				case <-returned:
				default:
					t.Error("pool closed before loader returned")
				}
				if lease.Engine.SparsePostingsReady() {
					t.Error("pool closed before engine")
				}
				closePool()
				close(poolClosed)
			}
			r.mu.Unlock()
			if !lastLease {
				lease.Done()
			}
			store := r.Auth.(*fakeStore)
			store.mu.Lock()
			store.active = ""
			store.mu.Unlock()
			activate := func() error { _, err := r.Resolve(t.Context(), "vn", rel); return err }
			result := make(chan error, 1)
			if lastLease {
				if err := activate(); !errors.Is(err, ErrExpired) {
					t.Fatal(err)
				}
				go func() { lease.Done(); result <- nil }()
			} else {
				go func() { result <- activate() }()
			}
			select {
			case err := <-result:
				if !lastLease && !errors.Is(err, ErrExpired) {
					t.Fatal(err)
				}
			case <-time.After(100 * time.Millisecond):
				t.Fatal("request waited for blocked loader")
			}
			finished := make(chan struct{})
			go func() { r.Close(); close(finished) }()
			select {
			case <-finished:
				t.Error("Close returned before loader cleanup")
			case <-time.After(20 * time.Millisecond):
			}
			stop()
			<-finished
			<-poolClosed
		})
	}
}
