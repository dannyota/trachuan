// Package cache supplies memory-only, byte-bounded least-recently-used caches.
package cache

// Stats contains cumulative counters and the current charged byte count.
// It deliberately carries no keys, values or account identifiers.
type Stats struct {
	Hits      uint64
	Misses    uint64
	Evictions uint64
	Bytes     int64
}
