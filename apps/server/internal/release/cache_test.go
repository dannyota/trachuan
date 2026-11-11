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

func TestResolver_ClosesRetiredPoolsOnNextResolve(t *testing.T) {
	r, s, _ := resolverFixture()
	var closed []string
	r.CloseDatabase = func(name string) { closed = append(closed, name) }
	for _, rel := range []string{"", "20990102"} {
		if _, _, err := r.Resolve(context.Background(), "vn", rel, false); err != nil {
			t.Fatal(err)
		}
	}
	s.retained = nil
	if _, _, err := r.Resolve(context.Background(), "vn", "", false); err != nil {
		t.Fatal(err)
	}
	if len(closed) != 1 || closed[0] != "corpus_vn_20990102" {
		t.Fatalf("closed=%v", closed)
	}
	r.Close()
	r.Close()
	if len(closed) != 2 || closed[1] != "corpus_vn_20990103" {
		t.Fatalf("closed=%v", closed)
	}
	if _, _, err := r.Resolve(context.Background(), "vn", "", false); err == nil {
		t.Fatal("resolved after close")
	}
}
func TestResolver_ReadThenSearchUsesEachOpenerOnce(t *testing.T) {
	r, _, engine := resolverFixture()
	reads, searches := 0, 0
	r.OpenRead = func(string) (*retrieval.Engine, error) { reads++; return engine, nil }
	r.Open = func(string) (*retrieval.Engine, error) { searches++; return engine, nil }
	for _, search := range []bool{false, true, false, true} {
		if _, _, err := r.Resolve(context.Background(), "vn", "", search); err != nil {
			t.Fatal(err)
		}
	}
	if reads != 1 || searches != 1 {
		t.Fatalf("reads=%d searches=%d", reads, searches)
	}
}

type observedContext struct {
	context.Context
	observed chan struct{}
	once     sync.Once
}

func (ctx *observedContext) Done() <-chan struct{} {
	ctx.once.Do(func() { close(ctx.observed) })
	return ctx.Context.Done()
}

func TestResolver_ConcurrentReadSurvivesIncompatibleSearch(t *testing.T) {
	r, _, engine := resolverFixture()
	entered, proceed := make(chan struct{}), make(chan struct{})
	r.Open = func(string) (*retrieval.Engine, error) {
		close(entered)
		<-proceed
		return nil, errors.New("invalid dense recipe")
	}
	searchResult := make(chan error, 1)
	go func() { _, _, err := r.Resolve(context.Background(), "vn", "20990102", true); searchResult <- err }()
	<-entered
	ctx := &observedContext{Context: context.Background(), observed: make(chan struct{})}
	readResult := make(chan error, 1)
	go func() {
		got, _, err := r.Resolve(ctx, "vn", "20990102", false)
		if err == nil && got != engine {
			err = errors.New("wrong engine")
		}
		readResult <- err
	}()
	<-ctx.observed
	close(proceed)
	if err := <-searchResult; !errors.Is(err, ErrIncompatible) {
		t.Fatalf("search error=%v", err)
	}
	if err := <-readResult; err != nil {
		t.Fatalf("read error=%v", err)
	}
}
func TestResolver_EvictionDuringOpenClosesAndRejectsEngine(t *testing.T) {
	r, s, engine := resolverFixture()
	entered, proceed := make(chan struct{}), make(chan struct{})
	r.Open = func(name string) (*retrieval.Engine, error) {
		if name == "corpus_vn_20990102" {
			close(entered)
			<-proceed
		}
		return engine, nil
	}
	var mu sync.Mutex
	var closed []string
	r.CloseDatabase = func(name string) { mu.Lock(); defer mu.Unlock(); closed = append(closed, name) }
	result := make(chan error, 1)
	go func() { _, _, err := r.Resolve(context.Background(), "vn", "20990102", true); result <- err }()
	<-entered
	s.mu.Lock()
	s.retained = nil
	s.mu.Unlock()
	if _, _, err := r.Resolve(context.Background(), "vn", "", true); err != nil {
		t.Fatal(err)
	}
	close(proceed)
	if err := <-result; !errors.Is(err, ErrExpired) {
		t.Fatalf("pending error %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(closed) != 1 || closed[0] != "corpus_vn_20990102" {
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
func TestResolver_ExpiredEngineIsEvictedByClock(t *testing.T) {
	r, _, _ := resolverFixture()
	var closed bool
	r.CloseDatabase = func(string) { closed = true }
	if _, _, err := r.Resolve(context.Background(), "vn", "20990102", false); err != nil {
		t.Fatal(err)
	}
	now := r.Clock().Add(time.Hour)
	r.Clock = func() time.Time { return now }
	if _, _, err := r.Resolve(context.Background(), "vn", "20990102", false); !errors.Is(err, ErrExpired) {
		t.Fatal(err)
	}
	if !closed {
		t.Fatal("expired pool left open")
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
	go func() { _, _, err := r.Resolve(context.Background(), "vn", "", true); result <- err }()
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
func TestResolver_NilSearchEnginePreservesReadCache(t *testing.T) {
	r, _, engine := resolverFixture()
	var reads, closed int
	r.OpenRead = func(string) (*retrieval.Engine, error) { reads++; return engine, nil }
	r.CloseDatabase = func(string) { closed++ }
	if _, _, err := r.Resolve(context.Background(), "vn", "", false); err != nil {
		t.Fatal(err)
	}
	r.Open = func(string) (*retrieval.Engine, error) { return nil, nil }
	if _, _, err := r.Resolve(context.Background(), "vn", "", true); err == nil {
		t.Fatal("accepted nil engine")
	}
	got, _, err := r.Resolve(context.Background(), "vn", "", false)
	if err != nil || got != engine || reads != 1 || closed != 0 {
		t.Fatalf("engine=%p reads=%d closed=%d error=%v", got, reads, closed, err)
	}
}
func TestResolver_CanceledWaiterDoesNotCancelOtherOpen(t *testing.T) {
	r, _, engine := resolverFixture()
	entered, proceed := make(chan struct{}), make(chan struct{})
	r.Open = func(string) (*retrieval.Engine, error) { close(entered); <-proceed; return engine, nil }
	result := make(chan error, 1)
	go func() { _, _, err := r.Resolve(context.Background(), "vn", "", true); result <- err }()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := r.Resolve(ctx, "vn", "", true); !errors.Is(err, context.Canceled) {
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
		if _, _, err := r.Resolve(context.Background(), "vn", "", false); err != nil {
			t.Fatal(err)
		}
		s.mu.Lock()
		s.active = "corpus_vn_20990103"
		s.mu.Unlock()
		if _, _, err := r.Resolve(context.Background(), "vn", "unknown", false); !errors.Is(err, ErrExpired) {
			t.Fatal(err)
		}
	}
	got, rel, err := r.Resolve(ctx, "vn", "", true)
	if err != nil || got != engine || rel != "20990103" {
		t.Fatalf("engine=%p release=%q error=%v", got, rel, err)
	}
}

func TestResolver_CachedPoolFailureDoesNotReturnEngine(t *testing.T) {
	r, _, _ := resolverFixture()
	if _, _, err := r.Resolve(context.Background(), "vn", "", false); err != nil {
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
	if engine, _, resolveErr := r.Resolve(context.Background(), "vn", "", false); resolveErr == nil || engine != nil {
		t.Fatalf("engine=%p error=%v", engine, resolveErr)
	}
}
