package mcpapi

import (
	"context"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/cache"
	"danny.vn/trachuan/apps/server/internal/release"
	"danny.vn/trachuan/internal/retrieval"
)

type leasePointer struct{ active string }

func (s *leasePointer) GetActiveRelease(context.Context, string) (string, error) {
	return s.active, nil
}

func TestCache_PausedResolverLeaseCannotAdoptRollbackGeneration(t *testing.T) {
	caches := NewCaches(time.Now)
	pointer := &leasePointer{active: "corpus_vn_20990101"}
	resolver := &release.Resolver{Auth: pointer, Clock: time.Now, OnActivate: caches.ActivateRelease, Open: func(context.Context, string) (*retrieval.Engine, func(), error) { return &retrieval.Engine{}, nil, nil }}
	defer resolver.Close()
	take := func() *release.Lease {
		t.Helper()
		lease, err := resolver.Resolve(t.Context(), "vn", "")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(lease.Done)
		return lease
	}
	old := take() // Pause after lease acquisition and before any cache handle capture.
	pointer.active = "corpus_vn_20990102"
	take().Done()
	pointer.active = "corpus_vn_20990101"
	fresh := take()
	if old.Generation != 1 || fresh.Generation != 3 {
		t.Fatal("unexpected resolver observations", old.Generation, fresh.Generation)
	}
	late := caches.Scope(old.Release, old.Generation)
	current := caches.Scope(fresh.Release, fresh.Generation)
	staleHandles := []cache.Handle[[]byte]{late.Search, late.Documents, late.Sequences}
	freshHandles := []cache.Handle[[]byte]{current.Search, current.Documents, current.Sequences}
	for i, h := range freshHandles {
		h.Put(fresh.Release, [32]byte{}, []byte("fresh"))
		if _, hit := staleHandles[i].Get(old.Release, [32]byte{}); hit {
			t.Fatal("paused A lease read the rollback cache")
		}
		staleHandles[i].Put(old.Release, [32]byte{}, []byte("stale"))
		if got, ok := h.Get(fresh.Release, [32]byte{}); !ok || string(got) != "fresh" {
			t.Fatal("paused A lease wrote the rollback cache")
		}
	}
}
