// Package limits enforces in-memory per-account request limits.
package limits

import (
	"container/list"
	"sync"
	"time"
)

// Class identifies a request limit.
type Class uint8

const (
	// Search limits search requests.
	Search Class = iota
	// Read limits read requests and other tools.
	Read
)

const (
	maxBuckets = 100000
	idleLimit  = 10 * time.Minute
	capacity   = time.Minute
)

type key struct {
	accountID int64
	class     Class
}

type bucket struct {
	key     key
	credit  time.Duration
	updated time.Time
	used    time.Time
}

// Limiter tracks request budgets for accounts in this process.
type Limiter struct {
	mu      sync.Mutex
	clock   func() time.Time
	buckets map[key]*list.Element
	recent  list.List
}

// New creates a limiter using clock to measure elapsed time.
func New(clock func() time.Time) *Limiter {
	return &Limiter{clock: clock, buckets: make(map[key]*list.Element)}
}

// Allow consumes a token or returns how long until one becomes available.
func (l *Limiter) Allow(accountID int64, class Class) (ok bool, retryAfter time.Duration) {
	var cost time.Duration
	switch class {
	case Search:
		cost = time.Second
	case Read:
		cost = 500 * time.Millisecond
	default:
		return false, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock()
	for e := l.recent.Back(); e != nil; e = l.recent.Back() {
		if now.Sub(e.Value.(*bucket).used) <= idleLimit {
			break
		}
		l.remove(e)
	}
	k := key{accountID: accountID, class: class}
	e := l.buckets[k]
	if e == nil {
		if len(l.buckets) == maxBuckets {
			var evict *list.Element
			for candidate := l.recent.Back(); candidate != nil; candidate = candidate.Prev() {
				b := candidate.Value.(*bucket)
				if now.Sub(b.updated) >= capacity-b.credit {
					evict = candidate
					break
				}
			}
			if evict == nil {
				return false, time.Second
			}
			l.remove(evict)
		}
		e = l.recent.PushFront(&bucket{key: k, credit: capacity, updated: now, used: now})
		l.buckets[k] = e
	}
	b := e.Value.(*bucket)
	if elapsed := now.Sub(b.updated); elapsed > 0 {
		if elapsed >= capacity-b.credit {
			b.credit = capacity
		} else {
			b.credit += elapsed
		}
		b.updated = now
	}
	b.used = now
	l.recent.MoveToFront(e)
	if b.credit < cost {
		return false, cost - b.credit
	}
	b.credit -= cost
	return true, 0
}

func (l *Limiter) remove(e *list.Element) {
	delete(l.buckets, e.Value.(*bucket).key)
	l.recent.Remove(e)
}
