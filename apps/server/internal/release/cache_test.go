package release

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestResolver_SwitchClosesOldPool(t *testing.T) {
	r, s, _ := resolverFixture()
	var opened, closed []string
	r.Open = func(name string) (*retrieval.Engine, error) {
		opened = append(opened, name)
		return &retrieval.Engine{}, nil
	}
	r.CloseDatabase = func(name string) { closed = append(closed, name) }
	if _, _, err := r.Resolve(context.Background(), "vn", ""); err != nil {
		t.Fatal(err)
	}
	s.active = "corpus_vn_20990104"
	if _, rel, err := r.Resolve(context.Background(), "vn", ""); err != nil || rel != "20990104" {
		t.Fatalf("release=%q error=%v", rel, err)
	}
	if len(opened) != 2 || opened[0] != "corpus_vn_20990103" || opened[1] != "corpus_vn_20990104" {
		t.Fatalf("opened=%v", opened)
	}
	if len(closed) != 1 || closed[0] != "corpus_vn_20990103" {
		t.Fatalf("closed=%v", closed)
	}
	if _, _, err := r.Resolve(context.Background(), "vn", "20990103"); !errors.Is(err, ErrExpired) {
		t.Fatalf("old release error=%v", err)
	}
	r.Close()
	r.Close()
	if len(closed) != 2 || closed[1] != "corpus_vn_20990104" {
		t.Fatalf("closed=%v", closed)
	}
	if _, _, err := r.Resolve(context.Background(), "vn", ""); err == nil {
		t.Fatal("resolved after close")
	}
}
func TestResolver_EvictionDuringOpenClosesAndRejectsEngine(t *testing.T) {
	r, s, engine := resolverFixture()
	entered, proceed := make(chan struct{}), make(chan struct{})
	r.Open = func(name string) (*retrieval.Engine, error) {
		if name == "corpus_vn_20990103" {
			close(entered)
			<-proceed
		}
		return engine, nil
	}
	var mu sync.Mutex
	var closed []string
	r.CloseDatabase = func(name string) { mu.Lock(); defer mu.Unlock(); closed = append(closed, name) }
	result := make(chan error, 1)
	go func() { _, _, err := r.Resolve(context.Background(), "vn", "20990103"); result <- err }()
	<-entered
	s.mu.Lock()
	s.active = "corpus_vn_20990104"
	s.mu.Unlock()
	if _, _, err := r.Resolve(context.Background(), "vn", ""); err != nil {
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
	r, _, engine := resolverFixture()
	entered, proceed := make(chan struct{}), make(chan struct{})
	var mu sync.Mutex
	opened, closed, premature := false, false, false
	r.Open = func(string) (*retrieval.Engine, error) {
		close(entered)
		<-proceed
		mu.Lock()
		opened = true
		mu.Unlock()
		return engine, nil
	}
	r.CloseDatabase = func(string) { mu.Lock(); defer mu.Unlock(); premature = !opened; closed = true }
	result := make(chan error, 1)
	go func() { _, _, err := r.Resolve(context.Background(), "vn", ""); result <- err }()
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
	r, _, engine := resolverFixture()
	entered, proceed := make(chan struct{}), make(chan struct{})
	r.Open = func(string) (*retrieval.Engine, error) { close(entered); <-proceed; return engine, nil }
	result := make(chan error, 1)
	go func() { _, _, err := r.Resolve(context.Background(), "vn", ""); result <- err }()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := r.Resolve(ctx, "vn", ""); !errors.Is(err, context.Canceled) {
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
	r, s, engine := resolverFixture()
	ctx := &beforeWaitContext{Context: context.Background()}
	ctx.beforeWait = func() {
		// Schedule two activations after the original open finishes but before its
		// caller consumes the result, including an eviction of the original entry.
		<-r.flight.DoChan("corpus_vn_20990103", func() (any, error) { return nil, nil })
		s.mu.Lock()
		s.active = "corpus_vn_20990104"
		s.mu.Unlock()
		if _, _, err := r.Resolve(context.Background(), "vn", ""); err != nil {
			t.Fatal(err)
		}
		s.mu.Lock()
		s.active = "corpus_vn_20990103"
		s.mu.Unlock()
		if _, _, err := r.Resolve(context.Background(), "vn", "unknown"); !errors.Is(err, ErrExpired) {
			t.Fatal(err)
		}
	}
	got, rel, err := r.Resolve(ctx, "vn", "")
	if err != nil || got != engine || rel != "20990103" {
		t.Fatalf("engine=%p release=%q error=%v", got, rel, err)
	}
}

func TestResolver_CachedPoolFailureDoesNotReturnEngine(t *testing.T) {
	r, _, _ := resolverFixture()
	if _, _, err := r.Resolve(context.Background(), "vn", ""); err != nil {
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
	if engine, _, resolveErr := r.Resolve(context.Background(), "vn", ""); resolveErr == nil || engine != nil {
		t.Fatalf("engine=%p error=%v", engine, resolveErr)
	}
}
