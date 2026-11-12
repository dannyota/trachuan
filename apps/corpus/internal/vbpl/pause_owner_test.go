package vbpl

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"golang.org/x/time/rate"
)

type failingPauseGate struct {
	*sourcegate.Memory
	writes atomic.Int32
	called chan struct{}
	allow  chan struct{}
}

func (g *failingPauseGate) Pause(ctx context.Context, id int64, source, reason, evidence string) error {
	g.writes.Add(1)
	select {
	case g.called <- struct{}{}:
	default:
	}
	select {
	case <-g.allow:
		return g.Memory.Pause(ctx, id, source, reason, evidence)
	case <-ctx.Done():
		return ctx.Err()
	default:
		return errors.New("pause write failed")
	}
}

func TestPause_WaitersRespectOwnDeadline(t *testing.T) {
	g := &failingPauseGate{Memory: sourcegate.NewMemory(), called: make(chan struct{}, 10), allow: make(chan struct{})}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer server.Close()
	c := NewWithBase(server.URL, "test", WithLimits(float64(rate.Inf), 2), WithBackoffs(time.Millisecond), WithGate(g))
	parent, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	parent = sourcegate.WithActivity(sourcegate.WithRunID(parent, 1))
	if _, _, err := c.Detail(parent, "1"); !errors.Is(err, sourcegate.ErrSourcePaused) {
		t.Fatalf("initial error = %v", err)
	}
	results := make(chan error, 2)
	for range 2 {
		go func() {
			ctx, stop := context.WithTimeout(parent, 5*time.Millisecond)
			defer stop()
			results <- c.awaitPause(ctx, 1)
		}()
	}
	for range 2 {
		select {
		case err := <-results:
			if !errors.Is(err, sourcegate.ErrSourcePaused) || errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("error = %v", err)
			}
		case <-time.After(25 * time.Millisecond):
			t.Fatal("waiter exceeded its deadline")
		}
	}
}

func TestPause_RequestDeadlineIncludesSlotWait(t *testing.T) {
	g := &failingPauseGate{Memory: sourcegate.NewMemory(), called: make(chan struct{}, 10), allow: make(chan struct{})}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer server.Close()
	c := NewWithBase(server.URL, "test", WithLimits(float64(rate.Inf), 1), WithBackoffs(), WithGate(g))
	c.timeout, c.requestBudget = 30*time.Millisecond, 80*time.Millisecond
	ctx, cancel := context.WithCancel(sourcegate.WithRunID(context.Background(), 1))
	defer cancel()
	ctx = sourcegate.WithActivity(ctx)
	if _, _, err := c.Detail(ctx, "1"); !errors.Is(err, sourcegate.ErrSourcePaused) {
		t.Fatalf("initial error = %v", err)
	}
	<-g.called
	c.slots <- struct{}{}
	go func() { time.Sleep(20 * time.Millisecond); <-c.slots }()
	started := time.Now()
	_, _, err := c.Detail(ctx, "2")
	if !errors.Is(err, sourcegate.ErrSourcePaused) || time.Since(started) > 75*time.Millisecond {
		t.Fatalf("error = %v, elapsed = %v", err, time.Since(started))
	}
}

func TestPause_OwnerRetriesAndStops(t *testing.T) {
	for _, succeed := range []bool{true, false} {
		t.Run(map[bool]string{true: "persists", false: "cancel"}[succeed], func(t *testing.T) {
			g := &failingPauseGate{Memory: sourcegate.NewMemory(), called: make(chan struct{}, 20), allow: make(chan struct{})}
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests.Add(1); w.WriteHeader(http.StatusForbidden) }))
			defer server.Close()
			c := NewWithBase(server.URL, "test", WithLimits(float64(rate.Inf), 1), WithBackoffs(time.Millisecond), WithGate(g))
			ctx, cancel := context.WithCancel(sourcegate.WithRunID(context.Background(), 1))
			defer cancel()
			ctx = sourcegate.WithActivity(ctx)
			result := make(chan error, 1)
			go func() { _, _, err := c.Detail(ctx, "1"); result <- err }()
			select {
			case err := <-result:
				if !errors.Is(err, sourcegate.ErrSourcePaused) {
					t.Fatalf("error = %v", err)
				}
			case <-time.After(20 * time.Millisecond):
				t.Fatal("403 request waited for pause")
			}
			select {
			case <-g.called:
			case <-time.After(20 * time.Millisecond):
				t.Fatal("owner did not try pause")
			}
			if succeed {
				close(g.allow)
				select {
				case <-g.called:
				case <-time.After(30 * time.Millisecond):
					t.Fatal("owner did not retry")
				}
				waitCtx, stop := context.WithTimeout(ctx, 40*time.Millisecond)
				defer stop()
				if err := c.awaitPause(waitCtx, 1); err != nil && !errors.Is(err, sourcegate.ErrSourcePaused) {
					t.Fatalf("await: %v", err)
				}
				if err := g.Admit(ctx, 1, "vbpl"); !errors.Is(err, sourcegate.ErrSourcePaused) {
					t.Fatalf("admit: %v", err)
				}
				if _, _, err := c.Detail(ctx, "2"); !errors.Is(err, sourcegate.ErrSourcePaused) || requests.Load() != 1 {
					t.Fatalf("later error = %v, requests = %d", err, requests.Load())
				}
			} else {
				cancel()
				select {
				case <-g.called:
				default:
				}
				before := g.writes.Load()
				time.Sleep(10 * time.Millisecond)
				if g.writes.Load() != before {
					t.Fatal("owner continued after cancellation")
				}
			}
		})
	}
}

func TestPause_OwnerGoroutineStopsWithActivity(t *testing.T) {
	g := &failingPauseGate{Memory: sourcegate.NewMemory(), called: make(chan struct{}, 10), allow: make(chan struct{})}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer server.Close()
	c := NewWithBase(server.URL, "test", WithLimits(float64(rate.Inf), 1), WithBackoffs(time.Millisecond), WithGate(g))
	ctx, cancel := context.WithCancel(sourcegate.WithRunID(context.Background(), 1))
	ctx = sourcegate.WithActivity(ctx)
	_, _, err := c.Detail(ctx, "1")
	if !errors.Is(err, sourcegate.ErrSourcePaused) {
		cancel()
		t.Fatalf("error = %v", err)
	}
	c.pauseMu.Lock()
	done := c.pauses[1].ownerDone
	c.pauseMu.Unlock()
	cancel()
	select {
	case <-done:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("pause owner goroutine did not stop")
	}
}
