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
	metadata     func(context.Context) (Release, error)
	open         func(context.Context, string) (Pool, error)
	clock        func() time.Time
	after        func(time.Duration, func())
	ctx          context.Context
	cancel       context.CancelFunc
	metadataSlot chan struct{}
	resources    chan struct{}
	mu           sync.Mutex
	sequence     uint64
	observed     uint64
	active       *entry
	entries      map[*entry]bool
	closed       bool
}

type entry struct {
	release  Release
	pool     Pool
	cache    *cache.LRU[[]byte]
	slot     chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
	ready    chan struct{}
	openErr  error
	reserved bool
	retired  bool
	refs     int
	close    sync.Once
}

func NewManager(metadata func(context.Context) (Release, error), open func(context.Context, string) (Pool, error), clock func() time.Time) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{metadata: metadata, open: open, clock: clock, ctx: ctx, cancel: cancel, metadataSlot: make(chan struct{}, 1), resources: make(chan struct{}, 2), entries: make(map[*entry]bool), after: func(d time.Duration, f func()) { time.AfterFunc(d, f) }}
}

func (m *Manager) Resolve(ctx context.Context) (*Lease, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	stop := context.AfterFunc(m.ctx, cancel)
	defer func() { stop(); cancel() }()
	observation, err := m.readMetadata(ctx)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	e, err := m.observe(observation)
	if err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-e.ctx.Done():
		return nil, ErrUnavailable
	case <-e.ready:
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || e.retired || m.active != e {
		return nil, ErrUnavailable
	}
	if e.openErr != nil {
		return nil, fmt.Errorf("open public release: %w", e.openErr)
	}
	e.refs++
	return &Lease{Release: e.release, manager: m, entry: e}, nil
}

func (m *Manager) observe(o observation) (*entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || o.sequence < m.observed {
		return nil, ErrUnavailable
	}
	m.observed = o.sequence
	release := o.release
	if o.err != nil || !regexp.MustCompile(`^corpus_vn_[0-9]{8}(_[0-9]+)?$`).MatchString(release.Database) || release.ID != strings.TrimPrefix(release.Database, "corpus_vn_") {
		m.retireLocked()
		if o.err != nil {
			return nil, fmt.Errorf("resolve public release: %w", o.err)
		}
		return nil, ErrUnavailable
	}
	if m.active == nil || m.active.release != release || m.active.openErr != nil {
		m.retireLocked()
		work, cancel := context.WithCancel(m.ctx)
		e := &entry{release: release, cache: cache.New("summary_rows", 16<<20, 0, m.clock, func(b []byte) int { return len(b) }), slot: make(chan struct{}, 1), ctx: work, cancel: cancel, ready: make(chan struct{})}
		m.active = e
		m.entries[e] = true
		go m.openEntry(e)
	}
	return m.active, nil
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
