package release

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"danny.vn/trachuan/internal/retrieval"
	"github.com/jackc/pgx/v5/pgxpool"
)

func withPoolCloser(r *Resolver, closePool func(string)) {
	open := r.Open
	r.Open = func(ctx context.Context, name string) (*retrieval.Engine, func(), error) {
		engine, _, err := open(ctx, name)
		return engine, func() { closePool(name) }, err
	}
}

func resolveAndFinish(ctx context.Context, r *Resolver, rel string) error {
	lease, err := r.Resolve(ctx, "vn", rel)
	if lease != nil {
		lease.Done()
	}
	return err
}

func TestResolver_SwitchClosesOldPool(t *testing.T) {
	r, s, _ := resolverFixture(t)
	var opened, closed []string
	r.Open = func(_ context.Context, name string) (*retrieval.Engine, func(), error) {
		opened = append(opened, name)
		return &retrieval.Engine{}, nil, nil
	}
	withPoolCloser(r, func(name string) { closed = append(closed, name) })
	takeLease(t, r, "").Done()
	s.active = "corpus_vn_20990104"
	lease := takeLease(t, r, "")
	if lease.Release != "20990104" {
		t.Fatalf("release=%q", lease.Release)
	}
	if len(opened) != 2 || opened[0] != "corpus_vn_20990103" || opened[1] != "corpus_vn_20990104" {
		t.Fatalf("opened=%v", opened)
	}
	if len(closed) != 1 || closed[0] != "corpus_vn_20990103" {
		t.Fatalf("closed=%v", closed)
	}
	if _, err := r.Resolve(context.Background(), "vn", "20990103"); !errors.Is(err, ErrExpired) {
		t.Fatalf("old release error=%v", err)
	}
	lease.Done()
	r.Close()
	r.Close()
	if len(closed) != 2 || closed[1] != "corpus_vn_20990104" {
		t.Fatalf("closed=%v", closed)
	}
	if _, err := r.Resolve(context.Background(), "vn", ""); err == nil {
		t.Fatal("resolved after close")
	}
}
func TestResolver_EvictionDuringOpenClosesAndRejectsEngine(t *testing.T) {
	r, s, engine := resolverFixture(t)
	entered, proceed := make(chan struct{}), make(chan struct{})
	r.Open = func(_ context.Context, name string) (*retrieval.Engine, func(), error) {
		if name == "corpus_vn_20990103" {
			close(entered)
			<-proceed
		}
		return engine, nil, nil
	}
	var mu sync.Mutex
	var closed []string
	withPoolCloser(r, func(name string) { mu.Lock(); defer mu.Unlock(); closed = append(closed, name) })
	result := make(chan error, 1)
	go func() { err := resolveAndFinish(context.Background(), r, "20990103"); result <- err }()
	<-entered
	s.mu.Lock()
	s.active = "corpus_vn_20990104"
	s.mu.Unlock()
	if err := resolveAndFinish(context.Background(), r, ""); err != nil {
		t.Fatal(err)
	}
	close(proceed)
	if err := <-result; !errors.Is(err, ErrExpired) {
		t.Fatalf("pending error %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(closed) != 1 || closed[0] != "corpus_vn_20990103" {
		t.Fatalf("closed=%v", closed)
	}
}
func TestPoolConfig_RestrictsDatabaseAndConnections(t *testing.T) {
	cfg, err := poolConfig("postgres://reader@localhost/{database}?sslmode=disable", "corpus_vn_20990103")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConnConfig.Database != "corpus_vn_20990103" || cfg.MaxConns != 4 || cfg.MinConns != 0 {
		t.Fatal("wrong pool configuration")
	}
	for _, template := range []string{"postgres://localhost/postgres", "postgres://{database}@localhost/postgres", "postgres://localhost/{database}?dbname=postgres", "postgres://localhost/{database}/{database}"} {
		if _, err := poolConfig(template, "corpus_vn_20990103"); err == nil {
			t.Fatalf("accepted %q", template)
		}
	}
}
func TestResolver_CloseWaitsForPendingOpenerToFinish(t *testing.T) {
	r, _, engine := resolverFixture(t)
	entered, proceed := make(chan struct{}), make(chan struct{})
	var mu sync.Mutex
	opened, closed, premature := false, false, false
	r.Open = func(context.Context, string) (*retrieval.Engine, func(), error) {
		close(entered)
		<-proceed
		mu.Lock()
		opened = true
		mu.Unlock()
		return engine, nil, nil
	}
	withPoolCloser(r, func(string) { mu.Lock(); defer mu.Unlock(); premature = !opened; closed = true })
	result := make(chan error, 1)
	go func() { err := resolveAndFinish(context.Background(), r, ""); result <- err }()
	<-entered
	r.Close()
	close(proceed)
	if err := <-result; !errors.Is(err, ErrExpired) {
		t.Fatalf("pending error=%v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !closed || premature {
		t.Fatalf("closed=%t before open finished=%t", closed, premature)
	}
}
func TestResolver_CanceledWaiterDoesNotCancelOtherOpen(t *testing.T) {
	r, _, engine := resolverFixture(t)
	entered, proceed := make(chan struct{}), make(chan struct{})
	r.Open = func(context.Context, string) (*retrieval.Engine, func(), error) {
		close(entered)
		<-proceed
		return engine, nil, nil
	}
	result := make(chan error, 1)
	go func() { err := resolveAndFinish(context.Background(), r, ""); result <- err }()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := resolveAndFinish(ctx, r, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiter error %v", err)
	}
	close(proceed)
	if err := <-result; err != nil {
		t.Fatalf("first caller error %v", err)
	}
}

type beforeWaitContext struct {
	context.Context
	once       sync.Once
	beforeWait func()
}

func (ctx *beforeWaitContext) Done() <-chan struct{} {
	ctx.once.Do(ctx.beforeWait)
	return ctx.Context.Done()
}

func TestResolver_ActivationChangesBeforeOpenResultIsConsumed(t *testing.T) {
	r, s, engine := resolverFixture(t)
	ctx := &beforeWaitContext{Context: context.Background()}
	ctx.beforeWait = func() {
		// Schedule two activations after the original open finishes but before its
		// caller consumes the result, including an eviction of the original entry.
		<-r.flight.DoChan("corpus_vn_20990103", func() (any, error) { return nil, nil })
		s.mu.Lock()
		s.active = "corpus_vn_20990104"
		s.mu.Unlock()
		if err := resolveAndFinish(context.Background(), r, ""); err != nil {
			t.Fatal(err)
		}
		s.mu.Lock()
		s.active = "corpus_vn_20990103"
		s.mu.Unlock()
		if err := resolveAndFinish(context.Background(), r, "unknown"); !errors.Is(err, ErrExpired) {
			t.Fatal(err)
		}
	}
	got, err := r.Resolve(ctx, "vn", "")
	if err != nil {
		t.Fatal(err)
	}
	defer got.Done()
	if got.Engine != engine || got.Release != "20990103" {
		t.Fatalf("lease=%+v", got)
	}
}

func TestResolver_CachedPoolFailureDoesNotReturnEngine(t *testing.T) {
	r, _, _ := resolverFixture(t)
	if err := resolveAndFinish(context.Background(), r, ""); err != nil {
		t.Fatal(err)
	}
	cfg, err := poolConfig("postgres://reader@localhost/{database}?sslmode=disable", "corpus_vn_20990103")
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.DialFunc = func(context.Context, string, string) (net.Conn, error) {
		t.Error("unexpected network access")
		return nil, errors.New("network disabled")
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
	r.entries["corpus_vn_20990103"].pool = pool
	if lease, resolveErr := r.Resolve(context.Background(), "vn", ""); resolveErr == nil || lease != nil {
		if lease != nil {
			lease.Done()
		}
		t.Fatalf("lease=%+v error=%v", lease, resolveErr)
	}
}

func TestResolver_SwitchPreservesInFlightWork(t *testing.T) {
	r, s, engine := resolverFixture(t)
	var closed []string
	withPoolCloser(r, func(name string) { closed = append(closed, name) })
	first := takeLease(t, r, "20990103")
	second := takeLease(t, r, "20990103")
	s.active = "corpus_vn_20990104"
	current := takeLease(t, r, "")
	if current.Release != "20990104" || first.Engine != engine || first.Release != "20990103" {
		t.Fatalf("original=%+v current=%+v", first, current)
	}
	if len(closed) != 0 {
		t.Fatalf("closed with two requests in flight: %v", closed)
	}
	if _, err := r.Resolve(context.Background(), "vn", "20990103"); !errors.Is(err, ErrExpired) {
		t.Fatalf("new lease on old release: %v", err)
	}
	first.Done()
	first.Done()
	if len(closed) != 0 {
		t.Fatalf("closed before last lease ended: %v", closed)
	}
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(second.Done)
	}
	wg.Wait()
	if len(closed) != 1 || closed[0] != "corpus_vn_20990103" {
		t.Fatalf("closed=%v", closed)
	}
}

func TestResolver_PointerChangesDuringOpenWithoutAnotherResolve(t *testing.T) {
	r, s, engine := resolverFixture(t)
	r.Open = func(context.Context, string) (*retrieval.Engine, func(), error) {
		s.mu.Lock()
		s.active = "corpus_vn_20990104"
		s.mu.Unlock()
		return engine, nil, nil
	}
	got, err := r.Resolve(context.Background(), "vn", "20990103")
	if got != nil {
		defer got.Done()
	}
	if !errors.Is(err, ErrExpired) || got != nil {
		t.Fatalf("lease=%+v error=%v, want release expired", got, err)
	}
}

type waitingContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (ctx *waitingContext) Done() <-chan struct{} {
	ctx.once.Do(func() { close(ctx.waiting) })
	return ctx.Context.Done()
}

func TestResolver_CanceledInitiatorDoesNotCancelSharedOpen(t *testing.T) {
	r, _, engine := resolverFixture(t)
	entered, proceed := make(chan struct{}), make(chan struct{})
	r.Open = func(ctx context.Context, _ string) (*retrieval.Engine, func(), error) {
		close(entered)
		<-proceed
		return engine, nil, ctx.Err()
	}
	initiator, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := make(chan error, 1)
	go func() { err := resolveAndFinish(initiator, r, ""); first <- err }()
	<-entered
	waiter := &waitingContext{Context: context.Background(), waiting: make(chan struct{})}
	second := make(chan error, 1)
	go func() {
		got, err := r.Resolve(waiter, "vn", "")
		if got != nil {
			defer got.Done()
			if got.Engine != engine {
				err = errors.New("wrong engine")
			}
		}
		second <- err
	}()
	<-waiter.waiting
	cancel()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("initiator error=%v", err)
	}
	close(proceed)
	if err := <-second; err != nil {
		t.Fatalf("other caller lost shared open: %v", err)
	}
}

func TestResolver_UnpinnedPointerChangeDuringOpenUsesNewRelease(t *testing.T) {
	r, s, _ := resolverFixture(t)
	var opened []string
	r.Open = func(_ context.Context, name string) (*retrieval.Engine, func(), error) {
		opened = append(opened, name)
		if name == "corpus_vn_20990103" {
			s.mu.Lock()
			s.active = "corpus_vn_20990104"
			s.mu.Unlock()
		}
		return &retrieval.Engine{}, nil, nil
	}
	lease := takeLease(t, r, "")
	if lease.Release != "20990104" || len(opened) != 2 || opened[1] != "corpus_vn_20990104" {
		t.Fatalf("release=%q opened=%v", lease.Release, opened)
	}
}

func TestResolver_AuthFailureAfterOpenDoesNotLeaseEngine(t *testing.T) {
	r, s, engine := resolverFixture(t)
	failure := errors.New("auth down")
	r.Open = func(context.Context, string) (*retrieval.Engine, func(), error) {
		s.mu.Lock()
		s.err = failure
		s.mu.Unlock()
		return engine, nil, nil
	}
	lease, err := r.Resolve(context.Background(), "vn", "20990103")
	if lease != nil {
		defer lease.Done()
	}
	if lease != nil || !errors.Is(err, failure) {
		t.Fatalf("lease=%+v error=%v", lease, err)
	}
}

func TestResolver_SharedOpenHasTenSecondBound(t *testing.T) {
	r, _, engine := resolverFixture(t)
	r.Open = func(ctx context.Context, _ string) (*retrieval.Engine, func(), error) {
		deadline, ok := ctx.Deadline()
		remaining := time.Until(deadline)
		if !ok || remaining <= 0 || remaining > 10*time.Second {
			return nil, nil, errors.New("shared open has no ten-second bound")
		}
		return engine, nil, nil
	}
	takeLease(t, r, "")
}

func TestResolver_CloseCancelsSharedOpen(t *testing.T) {
	r, _, _ := resolverFixture(t)
	entered := make(chan struct{})
	r.Open = func(ctx context.Context, _ string) (*retrieval.Engine, func(), error) {
		close(entered)
		<-ctx.Done()
		return nil, nil, ctx.Err()
	}
	result := make(chan error, 1)
	go func() { result <- resolveAndFinish(context.Background(), r, "") }()
	<-entered
	r.Close()
	if err := <-result; !errors.Is(err, ErrExpired) {
		t.Fatalf("resolve after shutdown: %v", err)
	}
}

func TestResolver_CloseDefersPoolUntilLeaseEnds(t *testing.T) {
	r, _, _ := resolverFixture(t)
	closed := 0
	withPoolCloser(r, func(string) { closed++ })
	lease := takeLease(t, r, "")
	r.Close()
	if closed != 0 {
		t.Fatal("shutdown closed a leased pool")
	}
	lease.Done()
	lease.Done()
	if closed != 1 {
		t.Fatalf("closed=%d", closed)
	}
}
