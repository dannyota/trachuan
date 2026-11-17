package publicsummary

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"danny.vn/trachuan/apps/server/internal/cache"
)

type Pool interface {
	Row(context.Context, string) (Row, error)
	List(context.Context) ([]Item, error)
	Close()
}

type Manager struct {
	metadata func(context.Context) (Release, error)
	open     func(context.Context, string) (Pool, error)
	clock    func() time.Time
	after    func(time.Duration, func())
	mu       sync.Mutex
	active   *entry
	entries  map[*entry]bool
	closed   bool
}

type entry struct {
	release Release
	pool    Pool
	cache   *cache.LRU[[]byte]
	slot    chan struct{}
	ctx     context.Context
	cancel  context.CancelFunc
	retired bool
	refs    int
	close   sync.Once
}

func NewManager(metadata func(context.Context) (Release, error), open func(context.Context, string) (Pool, error), clock func() time.Time) *Manager {
	return &Manager{metadata: metadata, open: open, clock: clock, entries: make(map[*entry]bool), after: func(d time.Duration, f func()) { time.AfterFunc(d, f) }}
}

func (m *Manager) Resolve(ctx context.Context) (*Lease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	release, err := m.metadata(ctx)
	if err != nil || !regexp.MustCompile(`^corpus_vn_[0-9]{8}(_[0-9]+)?$`).MatchString(release.Database) || release.ID != strings.TrimPrefix(release.Database, "corpus_vn_") {
		if ctx.Err() == nil {
			m.retireLocked()
		}
		if err != nil {
			return nil, fmt.Errorf("resolve public release: %w", err)
		}
		return nil, ErrUnavailable
	}
	if m.active == nil || m.active.release != release {
		m.retireLocked()
		var pool Pool
		if release.Projection == 1 {
			pool, err = m.open(ctx, release.Database)
			if err != nil {
				return nil, fmt.Errorf("open public release: %w", err)
			}
		}
		work, cancelWork := context.WithCancel(context.Background())
		m.active = &entry{release: release, pool: pool, cache: cache.New("summary_rows", 16<<20, 0, m.clock, func(b []byte) int { return len(b) }), slot: make(chan struct{}, 1), ctx: work, cancel: cancelWork}
		m.entries[m.active] = true
	}
	m.active.refs++
	return &Lease{Release: release, manager: m, entry: m.active}, nil
}

func (m *Manager) retireLocked() {
	old := m.active
	if old == nil {
		return
	}
	old.retired = true
	old.cache.Purge()
	m.active = nil
	if old.refs == 0 {
		m.closeEntry(old)
		return
	}
	m.after(5*time.Second, func() { m.mu.Lock(); defer m.mu.Unlock(); m.closeEntry(old) })
}

func (m *Manager) closeEntry(e *entry) {
	e.close.Do(func() {
		e.cancel()
		if e.pool != nil {
			e.pool.Close()
		}
	})
	delete(m.entries, e)
}

func (m *Manager) Watch(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	m.watch(ctx, ticker.C)
}

func (m *Manager) watch(ctx context.Context, ticks <-chan time.Time) {
	for {
		lease, err := m.Resolve(ctx)
		if err == nil {
			lease.Done()
		}
		select {
		case <-ctx.Done():
			return
		case <-ticks:
		}
	}
}

func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	m.active = nil
	for e := range m.entries {
		e.retired = true
		e.cache.Purge()
		m.closeEntry(e)
	}
}
