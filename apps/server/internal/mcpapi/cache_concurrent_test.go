package mcpapi

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/cache"
	"danny.vn/trachuan/internal/retrieval"
)

type cachedEngine struct {
	Engine
	release string
	calls   atomic.Int64
	entered chan struct{}
	unblock chan struct{}
}

func (e *cachedEngine) Search(ctx context.Context, _ retrieval.Request) (retrieval.Response, error) {
	e.calls.Add(1)
	if e.entered != nil {
		e.entered <- struct{}{}
		select {
		case <-e.unblock:
		case <-ctx.Done():
			return retrieval.Response{}, ctx.Err()
		}
	}
	return retrieval.Response{}, nil
}
func (e *cachedEngine) Present(context.Context, retrieval.Response, retrieval.Detail) (retrieval.SearchResult, error) {
	return retrieval.SearchResult{Release: e.release}, nil
}

type cachedResolver struct{ engine *cachedEngine }

func (r cachedResolver) Resolve(context.Context, string, string) (Engine, string, func(), error) {
	return r.engine, r.engine.release, func() {}, nil
}

func TestSearchCache_ConcurrentMissesComputeIndependently(t *testing.T) {
	e := &cachedEngine{release: "20990101", entered: make(chan struct{}, 2), unblock: make(chan struct{})}
	c := NewCaches(time.Now)
	c.ActivateRelease(e.release)
	cfg := Config{Resolver: cachedResolver{e}, Caches: c, Clock: time.Now}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	run := func(ctx context.Context, ch chan error) {
		_, _, err := cfg.cachedSearch(ctx, searchInput{Country: "vn", Query: "ngân hàng"}, func(any) error { return nil })
		ch <- err
	}
	a, b := make(chan error, 1), make(chan error, 1)
	go run(ctx, a)
	go run(context.Background(), b)
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
		t.Fatal("request did not cancel independently", err)
	}
	e.unblock <- struct{}{}
	if err := <-b; err != nil {
		t.Fatal("one caller canceled another", err)
	}
	if e.calls.Load() != 2 || c.Search.Stats().Bytes == 0 {
		t.Fatal("independent miss did not store")
	}
}

func TestSearchCache_LeaseReleaseAndAllCachesPurged(t *testing.T) {
	e := &cachedEngine{release: "20990101"}
	c := NewCaches(time.Now)
	c.ActivateRelease(e.release)
	cfg := Config{Resolver: cachedResolver{e}, Caches: c, Clock: time.Now}
	in := searchInput{Country: "vn", Query: "ngân hàng"}
	run := func() string {
		t.Helper()
		result, _, err := cfg.cachedSearch(context.Background(), in, func(any) error { return nil })
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		_ = json.Unmarshal(result.StructuredContent.(json.RawMessage), &out)
		return out["release"].(string)
	}
	if run() != e.release {
		t.Fatal("wrong release")
	}
	c.Scope(e.release).Documents.Put(e.release, [32]byte{}, []byte("metadata"))
	c.Scope(e.release).Sequences.Put(e.release, [32]byte{}, []byte("sequence"))
	e.release = "20990102"
	c.ActivateRelease(e.release)
	if c.Search.Stats().Bytes != 0 || c.Documents.Stats().Bytes != 0 || c.Sequences.Stats().Bytes != 0 {
		t.Fatal("activation retained corpus data")
	}
	miss, hit := run(), run()
	if miss != e.release || hit != e.release || e.calls.Load() != 2 {
		t.Fatal("new lease missed or returned old bytes")
	}
}

func TestCache_AllHandlesExpireAcrossRollback(t *testing.T) {
	c := NewCaches(time.Now)
	c.ActivateRelease("A")
	old := c.Scope("A")
	c.ActivateRelease("B")
	c.ActivateRelease("A")
	fresh := c.Scope("A")
	oldHandles := []cache.Handle[[]byte]{old.Search, old.Documents, old.Sequences}
	freshHandles := []cache.Handle[[]byte]{fresh.Search, fresh.Documents, fresh.Sequences}
	for i, h := range freshHandles {
		h.Put("A", [32]byte{}, []byte("fresh"))
		if _, ok := oldHandles[i].Get("A", [32]byte{}); ok {
			t.Fatal("old lease read a new activation")
		}
		oldHandles[i].Put("A", [32]byte{}, []byte("old"))
		if oldHandles[i].MaxValueBytes() != 0 {
			t.Fatal("old handle admits cache preparation")
		}
		if got, ok := h.Get("A", [32]byte{}); !ok || string(got) != "fresh" {
			t.Fatal("rollback handle did not use the cache normally")
		}
	}
}
