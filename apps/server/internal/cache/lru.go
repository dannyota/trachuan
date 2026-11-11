package cache

import (
	"container/list"
	"sync"
	"time"
)

type entry[V any] struct {
	key   [32]byte
	value V
	born  time.Time
	bytes int64
}

// LRU bounds retained values by charged bytes. Values must be immutable while
// retained; callers copy mutable data at their ownership boundary.
type LRU[V any] struct {
	mu       sync.Mutex
	entries  map[[32]byte]*list.Element
	order    list.List
	capBytes int64
	maxAge   time.Duration
	clock    func() time.Time
	size     func(V) int
	stats    Stats
}

// New creates a cache. A zero maxAge keeps entries until eviction or purge.
// The name is for the caller's metric registration, never a key label.
func New[V any](_ string, capBytes int64, maxAge time.Duration, clock func() time.Time, size func(V) int) *LRU[V] {
	if clock == nil {
		clock = time.Now
	}
	return &LRU[V]{entries: make(map[[32]byte]*list.Element), capBytes: capBytes, maxAge: maxAge, clock: clock, size: size}
}

// Get returns an unexpired value and refreshes its LRU position, not its age.
func (c *LRU[V]) Get(key [32]byte) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el := c.entries[key]; el != nil {
		e := el.Value.(entry[V])
		if c.maxAge > 0 && !c.clock().Before(e.born.Add(c.maxAge)) {
			c.remove(el)
		} else {
			c.order.MoveToFront(el)
			c.stats.Hits++
			return e.value, true
		}
	}
	c.stats.Misses++
	var zero V
	return zero, false
}

// Put retains v only when its value size is at most one sixteenth of the cap.
func (c *LRU[V]) Put(key [32]byte, v V) {
	size := int64(c.size(v))
	c.mu.Lock()
	defer c.mu.Unlock()
	if size < 0 || c.capBytes <= 0 || size > c.capBytes/16 || size > c.capBytes-160 {
		return
	}
	if el := c.entries[key]; el != nil {
		c.stats.Bytes -= el.Value.(entry[V]).bytes
		c.order.Remove(el)
		delete(c.entries, key)
	}
	charged := size + 160
	for c.stats.Bytes > c.capBytes-charged {
		c.remove(c.order.Back())
	}
	el := c.order.PushFront(entry[V]{key: key, value: v, born: c.clock(), bytes: charged})
	c.entries[key] = el
	c.stats.Bytes += charged
}

func (c *LRU[V]) remove(el *list.Element) {
	e := el.Value.(entry[V])
	delete(c.entries, e.key)
	c.order.Remove(el)
	c.stats.Bytes -= e.bytes
	c.stats.Evictions++
}

// Purge releases every entry and preserves cumulative counters.
func (c *LRU[V]) Purge() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stats.Evictions += uint64(c.order.Len())
	c.stats.Bytes = 0
	clear(c.entries)
	c.order.Init()
}

// Stats returns a consistent snapshot without exposing cache contents.
func (c *LRU[V]) Stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stats
}

func (c *LRU[V]) recordMiss() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stats.Misses++
}

// MaxValueBytes is the largest value that fits both the oversized and byte caps.
func (c *LRU[V]) MaxValueBytes() int64 { return max(int64(0), min(c.capBytes/16, c.capBytes-160)) }
