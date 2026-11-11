package cache

import "sync"

// Scoped admits values only for the active release, including writes by
// requests that were still computing when activation purged the cache.
type Scoped[V any] struct {
	mu     sync.Mutex
	active string
	lru    *LRU[V]
}

// NewScoped creates a disabled release cache; call ActivateRelease before use.
func NewScoped[V any](lru *LRU[V]) *Scoped[V] { return &Scoped[V]{lru: lru} }

// ActivateRelease purges old values before admitting the new release.
// The resolver adapter must call this in switch order, before lending its lease.
func (c *Scoped[V]) ActivateRelease(release string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active != release {
		c.lru.Purge()
		c.active = release
	}
}

// Get returns only values belonging to the active release.
func (c *Scoped[V]) Get(release string, key [32]byte) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if release == "" || release != c.active {
		c.lru.recordMiss()
		var zero V
		return zero, false
	}
	return c.lru.Get(key)
}

// Put refuses writes from retired or unopened releases.
func (c *Scoped[V]) Put(release string, key [32]byte, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if release != "" && release == c.active {
		c.lru.Put(key, value)
	}
}

// Stats returns the underlying counters and current charged bytes.
func (c *Scoped[V]) Stats() Stats { return c.lru.Stats() }
