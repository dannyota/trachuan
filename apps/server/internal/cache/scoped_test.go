package cache

import (
	"testing"
	"time"
)

func TestScoped_RejectsRetiredLeaseWrites(t *testing.T) {
	c := NewScoped(New("test", 4096, 0, time.Now, func(s string) int { return len(s) }))
	c.ActivateRelease("old", 1)
	old := c.Scope("old", 1)
	old.Put("old", [32]byte{}, "value")
	c.ActivateRelease("new", 2)
	old.Put("old", [32]byte{}, "late")
	if c.Stats().Bytes != 0 {
		t.Fatal("retired request repopulated cache")
	}
	if _, ok := old.Get("old", [32]byte{}); ok {
		t.Fatal("retired cache hit")
	}
	if c.Stats().Misses != 1 {
		t.Fatal("retired lookup miss not counted")
	}
	fresh := c.Scope("new", 2)
	fresh.Put("new", [32]byte{}, "new")
	if _, ok := fresh.Get("new", [32]byte{}); !ok {
		t.Fatal("fresh handle missed")
	}
}

func TestScoped_OldLeaseCannotUseRollbackGeneration(t *testing.T) {
	c := NewScoped(New("test", 4096, 0, time.Now, func(s string) int { return len(s) }))
	c.ActivateRelease("A", 1)
	old := c.Scope("A", 1)
	c.ActivateRelease("B", 2)
	c.ActivateRelease("A", 3)
	fresh := c.Scope("A", 3)
	key := [32]byte{}
	fresh.Put("A", key, "fresh")
	if _, ok := old.Get("A", key); ok {
		t.Fatal("old A lease read the rollback generation")
	}
	old.Put("A", key, "old")
	if got, ok := fresh.Get("A", key); !ok || got != "fresh" {
		t.Fatal("old A lease wrote the rollback generation")
	}
}

func TestScoped_LateScopeRejectsPausedLeaseAcrossRollback(t *testing.T) {
	c := NewScoped(New("test", 4096, 0, time.Now, func(v string) int { return len(v) }))
	c.ActivateRelease("A", 1)
	// The lease is acquired now; handle capture is delayed until after rollback.
	lease := struct {
		release    string
		generation uint64
	}{"A", 1}
	c.ActivateRelease("B", 2)
	c.ActivateRelease("A", 3)
	fresh := c.Scope("A", 3)
	fresh.Put("A", [32]byte{}, "fresh")
	late := c.Scope(lease.release, lease.generation)
	if _, ok := late.Get("A", [32]byte{}); ok {
		t.Fatal("paused lease adopted the rollback generation")
	}
	late.Put("A", [32]byte{}, "old")
	if value, ok := fresh.Get("A", [32]byte{}); !ok || value != "fresh" {
		t.Fatal("paused lease overwrote fresh data")
	}
}

func TestScoped_GenerationMustMatchDeliveredNotification(t *testing.T) {
	c := NewScoped(New("test", 4096, 0, time.Now, func(v string) int { return len(v) }))
	c.ActivateRelease("A", 41)
	fresh := c.Scope("A", 41)
	fresh.Put("A", [32]byte{}, "fresh")
	for _, generation := range []uint64{0, 1, 40, 42} {
		h := c.Scope("A", generation)
		if _, ok := h.Get("A", [32]byte{}); ok {
			t.Fatal("mismatched generation hit", generation)
		}
		h.Put("A", [32]byte{}, "other")
		if h.MaxValueBytes() != 0 {
			t.Fatal("mismatched generation admitted preparation")
		}
	}
	if got, ok := fresh.Get("A", [32]byte{}); !ok || got != "fresh" {
		t.Fatal("resolver generation was not used verbatim")
	}
}
