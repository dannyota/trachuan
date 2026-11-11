package release

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"danny.vn/trachuan/internal/retrieval"
)

type fakeStore struct {
	mu     sync.Mutex
	active string
	err    error
}

func (s *fakeStore) GetActiveRelease(_ context.Context, country string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if country != "vn" {
		return "", nil
	}
	return s.active, s.err
}

func resolverFixture(t *testing.T) (*Resolver, *fakeStore, *retrieval.Engine) {
	t.Helper()
	now := time.Date(2099, 1, 3, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{active: "corpus_vn_20990103"}
	engine := &retrieval.Engine{}
	r := &Resolver{Auth: store, Clock: func() time.Time { return now }, Open: func(context.Context, string) (*retrieval.Engine, func(), error) { return engine, nil, nil }}
	t.Cleanup(r.Close)
	return r, store, engine
}

func takeLease(t *testing.T, r *Resolver, rel string) *Lease {
	t.Helper()
	lease, err := r.Resolve(context.Background(), "vn", rel)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(lease.Done)
	return lease
}

func TestResolver_ActiveByDefault(t *testing.T) {
	r, _, want := resolverFixture(t)
	var name string
	r.Open = func(_ context.Context, db string) (*retrieval.Engine, func(), error) {
		name = db
		return want, nil, nil
	}
	got := takeLease(t, r, "")
	if got.Engine != want || got.Release != "20990103" || name != "corpus_vn_20990103" {
		t.Fatalf("lease=%+v db=%q", got, name)
	}
}

func TestResolver_PinnedActiveResolves(t *testing.T) {
	r, _, want := resolverFixture(t)
	got := takeLease(t, r, "20990103")
	if got.Engine != want || got.Release != "20990103" {
		t.Fatalf("lease=%+v", got)
	}
}

func TestResolver_PinnedNonActiveIsExpired(t *testing.T) {
	for _, rel := range []string{"20990102", "20990101", "unknown", "corpus_vn_20990103", "../20990103"} {
		t.Run(rel, func(t *testing.T) {
			r, _, _ := resolverFixture(t)
			r.Open = func(context.Context, string) (*retrieval.Engine, func(), error) {
				t.Error("opened non-active database")
				return nil, nil, nil
			}
			got, err := r.Resolve(context.Background(), "vn", rel)
			if !errors.Is(err, ErrExpired) || got != nil {
				t.Fatalf("lease=%+v error=%v", got, err)
			}
		})
	}
}

func TestResolver_ConcurrentFirstUseOpensOnce(t *testing.T) {
	r, _, engine := resolverFixture(t)
	var count atomic.Int32
	entered, proceed := make(chan struct{}), make(chan struct{})
	r.Open = func(context.Context, string) (*retrieval.Engine, func(), error) {
		if count.Add(1) == 1 {
			close(entered)
		}
		<-proceed
		return engine, nil, nil
	}
	const clients = 24
	var wg sync.WaitGroup
	for range clients {
		wg.Go(func() {
			got, err := r.Resolve(context.Background(), "vn", "")
			if err != nil {
				t.Error(err)
				return
			}
			defer got.Done()
			if got.Engine != engine {
				t.Errorf("engine=%p", got.Engine)
			}
		})
	}
	<-entered
	close(proceed)
	wg.Wait()
	if count.Load() != 1 {
		t.Fatalf("opens=%d", count.Load())
	}
}

func TestResolver_FailedOpenIsRetried(t *testing.T) {
	r, _, engine := resolverFixture(t)
	attempts := 0
	r.Open = func(context.Context, string) (*retrieval.Engine, func(), error) {
		attempts++
		if attempts == 1 {
			return nil, nil, errors.New("temporary")
		}
		return engine, nil, nil
	}
	if _, err := r.Resolve(context.Background(), "vn", ""); err == nil {
		t.Fatal("accepted failed open")
	}
	got := takeLease(t, r, "")
	if got.Engine != engine || attempts != 2 {
		t.Fatalf("engine=%p attempts=%d", got.Engine, attempts)
	}
}

func TestResolver_RejectsUntrustedDatabaseNames(t *testing.T) {
	for _, name := range []string{"postgres", "corpus_vn_20990103;DROP", "corpus_us_20990103", "corpus_vn_20990103/other"} {
		t.Run(name, func(t *testing.T) {
			r, s, _ := resolverFixture(t)
			s.active = name
			r.Open = func(context.Context, string) (*retrieval.Engine, func(), error) {
				t.Fatal("opened untrusted name")
				return nil, nil, nil
			}
			if _, err := r.Resolve(context.Background(), "vn", ""); err == nil {
				t.Fatal("accepted name")
			}
		})
	}
}

func TestResolver_AuthFailureDoesNotServeCachedRelease(t *testing.T) {
	r, s, _ := resolverFixture(t)
	takeLease(t, r, "")
	s.err = errors.New("auth down")
	if _, err := r.Resolve(context.Background(), "vn", ""); err == nil {
		t.Fatal("served cached engine while auth down")
	}
}

func TestResolver_OpenFailureIsPreserved(t *testing.T) {
	r, _, _ := resolverFixture(t)
	failure := errors.New("invalid dense recipe")
	r.Open = func(context.Context, string) (*retrieval.Engine, func(), error) {
		return nil, nil, fmt.Errorf("open: %w", failure)
	}
	if _, err := r.Resolve(context.Background(), "vn", "20990103"); !errors.Is(err, failure) {
		t.Fatalf("open failure=%v", err)
	}
}
