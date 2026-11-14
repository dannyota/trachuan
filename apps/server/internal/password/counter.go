package password

import (
	"crypto/sha256"
	"sync"
	"time"
)

type attempts struct {
	start    time.Time
	count    int
	inFlight int
}
type Counter struct {
	mu         sync.Mutex
	clock      func() time.Time
	entries    map[[32]byte]*attempts
	maxEntries int
}

func NewCounter(clock func() time.Time) *Counter {
	return &Counter{clock: clock, entries: make(map[[32]byte]*attempts), maxEntries: 100000}
}
func (c *Counter) Reserve(address string) (func(bool), bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.clock()
	key := sha256.Sum256([]byte(address))
	entry, exists := c.entries[key]
	if !exists && len(c.entries) >= c.maxEntries {
		for k, e := range c.entries {
			if e.inFlight == 0 && !now.Before(e.start.Add(15*time.Minute)) {
				delete(c.entries, k)
			}
		}
		if len(c.entries) >= c.maxEntries {
			return nil, false
		}
	}
	if !exists {
		entry = &attempts{start: now}
	}
	if !now.Before(entry.start.Add(15 * time.Minute)) {
		entry.start = now
		entry.count = entry.inFlight
	}
	if entry.count >= 10 {
		return nil, false
	}
	entry.count++
	entry.inFlight++
	c.entries[key] = entry
	var once sync.Once
	return func(release bool) {
		once.Do(func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			entry.inFlight--
			if release {
				entry.count--
			}
			if entry.count == 0 {
				delete(c.entries, key)
			}
		})
	}, true
}
