package cache

import (
	"testing"
	"time"
)

func TestScoped_RejectsRetiredLeaseWrites(t *testing.T) {
	c := NewScoped(New("test", 4096, 0, time.Now, func(s string) int { return len(s) }))
	c.ActivateRelease("old")
	old := c.Scope("old")
	old.Put("old", [32]byte{}, "value")
	c.ActivateRelease("new")
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
	fresh := c.Scope("new")
	fresh.Put("new", [32]byte{}, "new")
	if _, ok := fresh.Get("new", [32]byte{}); !ok {
		t.Fatal("fresh handle missed")
	}
}

func TestScoped_OldLeaseCannotUseRollbackGeneration(t *testing.T) {
	c := NewScoped(New("test", 4096, 0, time.Now, func(s string) int { return len(s) }))
	c.ActivateRelease("A")
	old := c.Scope("A")
	c.ActivateRelease("B")
	c.ActivateRelease("A")
	fresh := c.Scope("A")
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
