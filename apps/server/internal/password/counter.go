package password

import (
	"crypto/sha256"
	"sync"
	"time"
)

type attempts struct {
	start time.Time
	count int
}
type Counter struct {
	mu         sync.Mutex
	clock      func() time.Time
	entries    map[[32]byte]attempts
	maxEntries int
}

func NewCounter(clock func() time.Time) *Counter {
	return &Counter{clock: clock, entries: make(map[[32]byte]attempts), maxEntries: 100000}
}
func (c *Counter) Allow(address string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.clock()
	key := sha256.Sum256([]byte(address))
	entry, exists := c.entries[key]
	if !exists && len(c.entries) >= c.maxEntries {
		for k, e := range c.entries {
			if !now.Before(e.start.Add(15 * time.Minute)) {
				delete(c.entries, k)
			}
		}
		if len(c.entries) >= c.maxEntries {
			return false
		}
	}
	if entry.start.IsZero() || !now.Before(entry.start.Add(15*time.Minute)) {
		entry = attempts{start: now}
	}
	if entry.count >= 10 {
		return false
	}
	entry.count++
	c.entries[key] = entry
	return true
}
