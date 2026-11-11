package cache

import (
	"testing"
	"time"
)

func TestScoped_RejectsRetiredLeaseWrites(t *testing.T) {
	c := NewScoped(New("test", 4096, 0, time.Now, func(s string) int { return len(s) }))
	c.ActivateRelease("old")
	c.Put("old", [32]byte{}, "value")
	c.ActivateRelease("new")
	c.Put("old", [32]byte{}, "late")
	if c.Stats().Bytes != 0 {
		t.Fatal("retired request repopulated cache")
	}
	if _, ok := c.Get("old", [32]byte{}); ok {
		t.Fatal("retired cache hit")
	}
	if c.Stats().Misses != 1 {
		t.Fatal("retired lookup miss not counted")
	}
	c.Put("new", [32]byte{}, "new")
	c.ActivateRelease("new")
	if _, ok := c.Get("new", [32]byte{}); !ok {
		t.Fatal("same release purged")
	}
}
