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
	"github.com/jackc/pgx/v5/pgconn"
)

type fakeStore struct {
	mu       sync.Mutex
	active   string
	retained []RetainedRelease
	err      error
}

func (s *fakeStore) GetActiveRelease(_ context.Context, country string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if country != "vn" {
		return "", nil
	}
	return s.active, s.err
}
func (s *fakeStore) ListRetainedReleases(_ context.Context, country string) ([]RetainedRelease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if country != "vn" {
		return nil, nil
	}
	return append([]RetainedRelease(nil), s.retained...), s.err
}
func resolverFixture() (*Resolver, *fakeStore, *retrieval.Engine) {
	now := time.Date(2099, 1, 3, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{active: "corpus_vn_20990103", retained: []RetainedRelease{{DBName: "corpus_vn_20990102", RetainUntil: now.Add(time.Hour)}}}
	engine := &retrieval.Engine{}
	r := &Resolver{Auth: store, Clock: func() time.Time { return now }, Open: func(string) (*retrieval.Engine, error) { return engine, nil }, OpenRead: func(string) (*retrieval.Engine, error) { return engine, nil }}
	return r, store, engine
}
func TestResolver_ActiveByDefault(t *testing.T) {
	r, _, want := resolverFixture()
	var name string
	r.Open = func(db string) (*retrieval.Engine, error) { name = db; return want, nil }
	got, rel, err := r.Resolve(context.Background(), "vn", "", true)
	if err != nil || got != want || rel != "20990103" || name != "corpus_vn_20990103" {
		t.Fatalf("engine=%p release=%q db=%q err=%v", got, rel, name, err)
	}
}
func TestResolver_RetainedWithinWindow(t *testing.T) {
	r, _, want := resolverFixture()
	got, rel, err := r.Resolve(context.Background(), "vn", "20990102", false)
	if err != nil || got != want || rel != "20990102" {
		t.Fatalf("%p %s %v", got, rel, err)
	}
}
func TestResolver_UnknownIsExpired(t *testing.T) {
	for _, rel := range []string{"20990101", "corpus_vn_20990102", "../20990102"} {
		t.Run(rel, func(t *testing.T) {
			r, _, _ := resolverFixture()
			_, _, err := r.Resolve(context.Background(), "vn", rel, false)
			if !errors.Is(err, ErrExpired) {
				t.Fatalf("error %v", err)
			}
		})
	}
}
func TestResolver_ExpiredAndDroppedAreExcluded(t *testing.T) {
	for _, dropped := range []bool{false, true} {
		r, s, _ := resolverFixture()
		row := &s.retained[0]
		if dropped {
			v := r.Clock()
			row.DroppedAt = &v
		} else {
			row.RetainUntil = r.Clock()
		}
		_, _, err := r.Resolve(context.Background(), "vn", "20990102", false)
		if !errors.Is(err, ErrExpired) {
			t.Fatalf("dropped=%t: %v", dropped, err)
		}
	}
}
func TestResolver_PrunedRetainedIsExpired(t *testing.T) {
	r, _, _ := resolverFixture()
	r.Open = func(string) (*retrieval.Engine, error) { return nil, &pgconn.PgError{Code: "3D000"} }
	r.OpenRead = r.Open
	_, _, err := r.Resolve(context.Background(), "vn", "20990102", true)
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("error %v", err)
	}
}
func TestResolver_IncompatibleSearchButReadable(t *testing.T) {
	r, _, want := resolverFixture()
	var reads int
	r.Open = func(string) (*retrieval.Engine, error) { return nil, errors.New("invalid dense recipe") }
	r.OpenRead = func(string) (*retrieval.Engine, error) { reads++; return want, nil }
	_, _, err := r.Resolve(context.Background(), "vn", "20990102", true)
	if !errors.Is(err, ErrIncompatible) {
		t.Fatalf("search error %v", err)
	}
	got, _, err := r.Resolve(context.Background(), "vn", "20990102", false)
	if err != nil || got != want || reads != 1 {
		t.Fatalf("read engine=%p opens=%d err=%v", got, reads, err)
	}
}
func TestResolver_ConcurrentFirstUseOpensOnce(t *testing.T) {
	r, _, engine := resolverFixture()
	var count atomic.Int32
	entered, proceed := make(chan struct{}), make(chan struct{})
	r.Open = func(string) (*retrieval.Engine, error) {
		if count.Add(1) == 1 {
			close(entered)
		}
		<-proceed
		return engine, nil
	}
	const clients = 24
	var wg sync.WaitGroup
	for range clients {
		wg.Go(func() {
			got, _, err := r.Resolve(context.Background(), "vn", "", true)
			if err != nil || got != engine {
				t.Errorf("%p %v", got, err)
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
	r, _, engine := resolverFixture()
	attempts := 0
	r.Open = func(string) (*retrieval.Engine, error) {
		attempts++
		if attempts == 1 {
			return nil, errors.New("temporary")
		}
		return engine, nil
	}
	r.OpenRead = func(string) (*retrieval.Engine, error) { return nil, errors.New("temporary") }
	if _, _, err := r.Resolve(context.Background(), "vn", "", true); err == nil {
		t.Fatal("accepted failed open")
	}
	got, _, err := r.Resolve(context.Background(), "vn", "", true)
	if err != nil || got != engine || attempts != 2 {
		t.Fatalf("%p attempts=%d err=%v", got, attempts, err)
	}
}
func TestResolver_RejectsUntrustedDatabaseNames(t *testing.T) {
	for _, name := range []string{"postgres", "corpus_vn_20990103;DROP", "corpus_us_20990103", "corpus_vn_20990103/other"} {
		t.Run(name, func(t *testing.T) {
			r, s, _ := resolverFixture()
			s.active = name
			r.Open = func(string) (*retrieval.Engine, error) { t.Fatal("opened untrusted name"); return nil, nil }
			if _, _, err := r.Resolve(context.Background(), "vn", "", true); err == nil {
				t.Fatal("accepted name")
			}
		})
	}
}
func TestResolver_AuthFailureDoesNotServeCachedRelease(t *testing.T) {
	r, s, _ := resolverFixture()
	if _, _, err := r.Resolve(context.Background(), "vn", "", true); err != nil {
		t.Fatal(err)
	}
	s.err = errors.New("auth down")
	if _, _, err := r.Resolve(context.Background(), "vn", "", true); err == nil {
		t.Fatal("served cached engine while auth down")
	}
}

func TestResolver_WrappedMismatchIsIncompatible(t *testing.T) {
	r, _, _ := resolverFixture()
	r.Open = func(string) (*retrieval.Engine, error) {
		return nil, fmt.Errorf("open search: %w", errors.New("invalid dense recipe"))
	}
	if _, _, err := r.Resolve(context.Background(), "vn", "20990102", true); !errors.Is(err, ErrIncompatible) {
		t.Fatalf("wrapped mismatch=%v", err)
	}
}
func TestResolver_EmbedderFailureIsNotIncompatible(t *testing.T) {
	r, _, _ := resolverFixture()
	failure := errors.New("read embedder recipe failed")
	r.Open = func(string) (*retrieval.Engine, error) { return nil, failure }
	r.OpenRead = func(string) (*retrieval.Engine, error) {
		t.Fatal("embedder outage treated as recipe incompatibility")
		return nil, nil
	}
	if _, _, err := r.Resolve(context.Background(), "vn", "20990102", true); !errors.Is(err, failure) {
		t.Fatalf("embedder failure=%v", err)
	}
}
