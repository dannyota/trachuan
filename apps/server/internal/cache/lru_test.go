package cache

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestLRU_ByteCapEvictsLeastRecentlyUsed(t *testing.T) {
	c := New("test", 2048, time.Hour, time.Now, func(v string) int { return len(v) })
	key := func(i int) [32]byte { return sha256.Sum256([]byte(fmt.Sprint(i))) }
	// Each one-byte value costs 32 + 1 + 128 = 161 bytes.
	for i := 0; i < 12; i++ {
		c.Put(key(i), "x")
	}
	c.Get(key(0))
	c.Put(key(12), "x")
	if _, ok := c.Get(key(1)); ok {
		t.Fatal("least recently used entry survived")
	}
	if _, ok := c.Get(key(0)); !ok {
		t.Fatal("recently used entry evicted")
	}
	if c.Stats().Bytes != 12*161 || c.Stats().Evictions != 1 {
		t.Fatal(c.Stats())
	}
	c.Put(key(0), "longer")
	if c.Stats().Bytes > 2048 {
		t.Fatal("replacement exceeded cap")
	}
}

func TestLRU_MaxAgeIsMiss(t *testing.T) {
	now := time.Now()
	c := New("test", 4096, time.Hour, func() time.Time { return now }, func(v string) int { return len(v) })
	c.Put([32]byte{}, "x")
	now = now.Add(59 * time.Minute)
	if _, ok := c.Get([32]byte{}); !ok {
		t.Fatal("expired early")
	}
	now = now.Add(time.Minute)
	if _, ok := c.Get([32]byte{}); ok {
		t.Fatal("age refreshed on hit")
	}
	if c.Stats().Bytes != 0 || c.Stats().Evictions != 1 {
		t.Fatal(c.Stats())
	}
}

func TestLRU_OversizedValueNotCached(t *testing.T) {
	c := New("test", 4096, 0, time.Now, func(v []byte) int { return len(v) })
	c.Put([32]byte{}, make([]byte, 257))
	if _, ok := c.Get([32]byte{}); ok {
		t.Fatal("oversized value cached")
	}
	c.Put([32]byte{}, make([]byte, 256))
	if _, ok := c.Get([32]byte{}); !ok {
		t.Fatal("boundary value rejected")
	}
}

func TestLRU_ConcurrentSafe(t *testing.T) {
	c := New("test", 4096, time.Hour, time.Now, func(v string) int { return len(v) })
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Go(func() {
			for j := 0; j < 100; j++ {
				c.Put([32]byte{}, "x")
				c.Get([32]byte{})
				c.Stats()
				if j%10 == 0 {
					c.Purge()
				}
			}
		})
	}
	wg.Wait()
	if c.Stats().Bytes > 4096 {
		t.Fatal(c.Stats())
	}
}

func TestLRU_PurgeAndStats(t *testing.T) {
	c := New("test", 4096, 0, time.Now, func(v string) int { return len(v) })
	c.Get([32]byte{})
	c.Put([32]byte{}, "x")
	c.Get([32]byte{})
	c.Purge()
	s := c.Stats()
	if s.Hits != 1 || s.Misses != 1 || s.Evictions != 1 || s.Bytes != 0 {
		t.Fatal(s)
	}
	if _, ok := c.Get([32]byte{}); ok {
		t.Fatal("purge retained value")
	}
}
