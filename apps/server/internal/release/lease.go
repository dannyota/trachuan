package release

import (
	"sync"

	"danny.vn/trachuan/internal/retrieval"
)

// Lease pins an engine and its pool for one request. Call Done when the request
// ends. A Lease must not be copied, and its Engine must not be used after Done.
type Lease struct {
	Engine  *retrieval.Engine
	Release string

	once sync.Once
	done func()
}

// Done releases the request's hold on the pool. Repeated calls have no effect.
func (l *Lease) Done() {
	l.once.Do(func() {
		if l.done != nil {
			l.done()
		}
	})
}

func (r *Resolver) finish(entry *cacheEntry) {
	r.mu.Lock()
	entry.leases--
	closePool := entry.retired && entry.leases == 0 && !entry.opening
	r.mu.Unlock()
	if closePool {
		entry.close()
	}
}
