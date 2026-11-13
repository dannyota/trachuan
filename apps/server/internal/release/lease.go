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
	var done <-chan struct{}
	r.mu.Lock()
	entry.leases--
	if r.closed && entry.leases == 0 && entry.opening {
		opening := entry.openingDone
		r.mu.Unlock()
		<-opening
		r.mu.Lock()
	}
	if entry.retired && entry.leases == 0 && !entry.opening {
		cleanup := r.cleanupLocked(entry)
		if r.closed {
			done = cleanup
		}
	}
	r.mu.Unlock()
	if done != nil {
		<-done
	}
}
