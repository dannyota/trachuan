package cache

import "sync"

// Scoped admits values only through handles from its current activation.
type Scoped[V any] struct {
	mu         sync.Mutex
	active     string
	generation uint64
	lru        *LRU[V]
}

// Handle binds cache access to one activation, including across rollbacks.
// Its zero value always misses and drops writes.
type Handle[V any] struct {
	cache      *Scoped[V]
	release    string
	generation uint64
}

// NewScoped creates a disabled cache; call ActivateRelease before taking scopes.
func NewScoped[V any](lru *LRU[V]) *Scoped[V] { return &Scoped[V]{lru: lru} }

// ActivateRelease purges values and advances the activation generation.
// The resolver must deliver activation notifications in pointer observation order.
func (c *Scoped[V]) ActivateRelease(release string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lru.Purge()
	c.active = release
	c.generation++
}

// Scope captures the current generation immediately after acquiring a lease.
func (c *Scoped[V]) Scope(release string) Handle[V] {
	c.mu.Lock()
	defer c.mu.Unlock()
	return Handle[V]{cache: c, release: release, generation: c.generation}
}

func (h Handle[V]) current(release string) bool {
	return release != "" && release == h.release && release == h.cache.active && h.generation == h.cache.generation
}

// Get misses when this handle belongs to an earlier activation.
func (h Handle[V]) Get(release string, key [32]byte) (V, bool) {
	if h.cache == nil {
		var zero V
		return zero, false
	}
	c := h.cache
	c.mu.Lock()
	defer c.mu.Unlock()
	if !h.current(release) {
		c.lru.recordMiss()
		var zero V
		return zero, false
	}
	return c.lru.Get(key)
}

// Put drops writes from an earlier activation, even if the release name matches.
func (h Handle[V]) Put(release string, key [32]byte, value V) {
	if h.cache == nil {
		return
	}
	c := h.cache
	c.mu.Lock()
	defer c.mu.Unlock()
	if h.current(release) {
		c.lru.Put(key, value)
	}
}

// Stats returns the underlying counters and current charged bytes.
func (c *Scoped[V]) Stats() Stats { return c.lru.Stats() }

// MaxValueBytes returns zero after this handle's activation ends.
func (h Handle[V]) MaxValueBytes() int64 {
	if h.cache == nil {
		return 0
	}
	c := h.cache
	c.mu.Lock()
	defer c.mu.Unlock()
	if !h.current(h.release) {
		return 0
	}
	return c.lru.MaxValueBytes()
}
