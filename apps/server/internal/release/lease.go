package release

import (
	"sync"

	"danny.vn/trachuan/internal/retrieval"
)

type Lease struct {
	Engine     *retrieval.Engine
	Release    string
	Generation uint64

	once sync.Once
	done func()
}

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
	if entry.retired && entry.leases == 0 && !entry.opening {
		r.cleanupLocked(entry)
	}
	r.mu.Unlock()
}
