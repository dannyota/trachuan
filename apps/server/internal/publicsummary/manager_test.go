package publicsummary

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type memoryPool struct {
	closed chan struct{}
	once   sync.Once
}

func (p *memoryPool) Close()                                 { p.once.Do(func() { close(p.closed) }) }
func (*memoryPool) Row(context.Context, string) (Row, error) { return Row{}, nil }
func (*memoryPool) List(context.Context) ([]Item, error)     { return []Item{}, nil }

func TestManager_RetiresOnActivationAndRollback(t *testing.T) {
	current := Release{Database: "corpus_vn_20990101", ID: "20990101", Projection: 1}
	var pools []*memoryPool
	var expire func()
	m := NewManager(func(context.Context) (Release, error) { return current, nil }, func(context.Context, string) (Pool, error) {
		p := &memoryPool{closed: make(chan struct{})}
		pools = append(pools, p)
		return p, nil
	}, time.Now)
	m.after = func(d time.Duration, f func()) {
		if d != 5*time.Second {
			t.Errorf("retirement: %v", d)
		}
		expire = f
	}
	defer m.Close()
	first, err := m.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	first.entry.cache.Put([32]byte{1}, []byte("old"))
	current = Release{Database: "corpus_vn_20990102", ID: "20990102", Projection: 1}
	second, err := m.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer second.Done()
	if _, err = first.Row(context.Background(), "vn:vbpl:1"); !errors.Is(err, ErrBusy) {
		t.Fatalf("retired row: %v", err)
	}
	if first.entry.cache.Stats().Bytes != 0 {
		t.Fatal("old cache retained")
	}
	select {
	case <-pools[0].closed:
		t.Fatal("closed before lease finished")
	default:
	}
	expire()
	select {
	case <-pools[0].closed:
	default:
		t.Fatal("old pool not closed at retirement deadline")
	}
	first.Done()
	current = Release{Database: "corpus_vn_20990101", ID: "20990101", Projection: 1}
	rollback, err := m.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer rollback.Done()
	if len(pools) != 3 || rollback.entry == first.entry {
		t.Fatal("rollback reused retired state")
	}
}

func TestManager_ResolutionFailureRetiresOldRelease(t *testing.T) {
	fail := false
	m := NewManager(func(context.Context) (Release, error) {
		if fail {
			return Release{}, errors.New("offline")
		}
		return Release{Database: "corpus_vn_20990101", ID: "20990101", Projection: 1}, nil
	}, func(context.Context, string) (Pool, error) { return &memoryPool{closed: make(chan struct{})}, nil }, time.Now)
	defer m.Close()
	old, err := m.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer old.Done()
	fail = true
	if _, err = m.Resolve(context.Background()); err == nil {
		t.Fatal("resolution failure ignored")
	}
	if _, err = old.Row(context.Background(), "vn:vbpl:1"); !errors.Is(err, ErrBusy) {
		t.Fatalf("stale release: %v", err)
	}
}

type blockingPool struct {
	memoryPool
	entered chan struct{}
	finish  chan struct{}
}

func (p *blockingPool) Row(ctx context.Context, key string) (Row, error) {
	close(p.entered)
	select {
	case <-ctx.Done():
		return Row{}, ctx.Err()
	case <-p.finish:
		return Row{Key: key, Payload: []byte(`{}`)}, nil
	}
}

func TestLease_NonWaitingAndCancellation(t *testing.T) {
	p := &blockingPool{memoryPool: memoryPool{closed: make(chan struct{})}, entered: make(chan struct{}), finish: make(chan struct{})}
	m := NewManager(func(context.Context) (Release, error) {
		return Release{Database: "corpus_vn_20990101", ID: "20990101", Projection: 1}, nil
	}, func(context.Context, string) (Pool, error) { return p, nil }, time.Now)
	defer m.Close()
	lease, err := m.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Done()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { _, e := lease.Row(ctx, "vn:vbpl:1"); result <- e }()
	<-p.entered
	if _, err = lease.List(context.Background()); !errors.Is(err, ErrBusy) {
		t.Fatalf("pool waited: %v", err)
	}
	cancel()
	if err = <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("disconnect: %v", err)
	}
	if _, err = lease.List(context.Background()); err != nil {
		t.Fatalf("slot not released: %v", err)
	}
}

func TestLease_CacheDropsAtRetirementAndRecomputesMisses(t *testing.T) {
	p := &memoryPool{closed: make(chan struct{})}
	current := Release{Database: "corpus_vn_20990101", ID: "20990101", Projection: 1}
	m := NewManager(func(context.Context) (Release, error) { return current, nil }, func(context.Context, string) (Pool, error) { return p, nil }, time.Now)
	defer m.Close()
	first, err := m.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer first.Done()
	if _, err = first.Row(context.Background(), "vn:vbpl:1"); err != nil {
		t.Fatal(err)
	}
	first.entry.slot <- struct{}{}
	if _, err = first.Row(context.Background(), "vn:vbpl:1"); err != nil {
		t.Fatalf("warm row touched busy database: %v", err)
	}
	if _, err = first.Row(context.Background(), "vn:vbpl:2"); !errors.Is(err, ErrBusy) {
		t.Fatalf("cold row skipped pool: %v", err)
	}
	<-first.entry.slot
	if stats := first.entry.cache.Stats(); stats.Hits != 1 || stats.Bytes == 0 || stats.Bytes > 16<<20 {
		t.Fatalf("cache: %+v", stats)
	}
	current.Projection = 0
	changed, err := m.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer changed.Done()
	if _, err = changed.Row(context.Background(), "vn:vbpl:1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("capability lost: %v", err)
	}
	if stats := first.entry.cache.Stats(); stats.Bytes != 0 {
		t.Fatalf("retired cache: %+v", stats)
	}
}

func TestManager_WatcherActivatesWithoutSummaryRequests(t *testing.T) {
	changes := make(chan Release, 2)
	opened := make(chan *memoryPool, 2)
	m := NewManager(func(ctx context.Context) (Release, error) {
		select {
		case r := <-changes:
			return r, nil
		case <-ctx.Done():
			return Release{}, ctx.Err()
		}
	}, func(context.Context, string) (Pool, error) {
		p := &memoryPool{closed: make(chan struct{})}
		opened <- p
		return p, nil
	}, time.Now)
	defer m.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := make(chan time.Time)
	done := make(chan struct{})
	changes <- Release{Database: "corpus_vn_20990101", ID: "20990101", Projection: 1}
	go func() { defer close(done); m.watch(ctx, ticks) }()
	first := <-opened
	changes <- Release{Database: "corpus_vn_20990102", ID: "20990102", Projection: 1}
	ticks <- time.Time{}
	<-opened
	select {
	case <-first.closed:
	default:
		t.Fatal("watcher kept old pool open")
	}
	cancel()
	<-done
}

func TestManager_RetirementCancelsInFlightQuery(t *testing.T) {
	current := Release{Database: "corpus_vn_20990101", ID: "20990101", Projection: 1}
	old := &blockingPool{memoryPool: memoryPool{closed: make(chan struct{})}, entered: make(chan struct{}), finish: make(chan struct{})}
	var expire func()
	m := NewManager(func(context.Context) (Release, error) { return current, nil }, func(_ context.Context, name string) (Pool, error) {
		if name == "corpus_vn_20990101" {
			return old, nil
		}
		return &memoryPool{closed: make(chan struct{})}, nil
	}, time.Now)
	m.after = func(_ time.Duration, f func()) { expire = f }
	defer m.Close()
	lease, err := m.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Done()
	result := make(chan error, 1)
	go func() { _, queryErr := lease.Row(context.Background(), "vn:vbpl:1"); result <- queryErr }()
	<-old.entered
	current = Release{Database: "corpus_vn_20990102", ID: "20990102", Projection: 1}
	next, err := m.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer next.Done()
	expire()
	if err = <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("retired query not cancelled: %v", err)
	}
	select {
	case <-old.closed:
	default:
		t.Fatal("retired pool not closed")
	}
}

func TestManager_CancelledRequestCannotRetireActiveRelease(t *testing.T) {
	release := Release{Database: "corpus_vn_20990101", ID: "20990101", Projection: 1}
	m := NewManager(func(ctx context.Context) (Release, error) {
		if err := ctx.Err(); err != nil {
			return Release{}, err
		}
		return release, nil
	}, func(context.Context, string) (Pool, error) { return &memoryPool{closed: make(chan struct{})}, nil }, time.Now)
	defer m.Close()
	active, err := m.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer active.Done()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = m.Resolve(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled resolution: %v", err)
	}
	if _, err = active.Row(context.Background(), "vn:vbpl:1"); err != nil {
		t.Fatalf("cancelled caller retired unrelated request: %v", err)
	}
}
