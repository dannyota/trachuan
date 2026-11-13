package vbpl

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
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

func forbiddenTransport(requests *atomic.Int32) http.RoundTripper {
	return statusTransport(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
}

func TestPause_RequestDeadlineIncludesSlotWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		g := &failingPauseGate{Memory: sourcegate.NewMemory(), called: make(chan struct{}, 10), allow: make(chan struct{})}
		var requests atomic.Int32
		c := NewWithBase("http://127.0.0.1", "test", WithTransport(forbiddenTransport(&requests)), WithLimits(float64(rate.Inf), 1), WithBackoffs(), WithGate(g))
		c.timeout, c.requestBudget = 30*time.Millisecond, 80*time.Millisecond
		ctx, cancel := context.WithCancel(sourcegate.WithRunID(context.Background(), 1))
		defer cancel()
		ctx = sourcegate.WithActivity(ctx)
		if _, _, err := c.Detail(ctx, "1"); !errors.Is(err, sourcegate.ErrSourcePaused) {
			t.Fatalf("initial error = %v", err)
		}
		<-g.called
		c.slots <- struct{}{}
		go func() { timer := time.NewTimer(20 * time.Millisecond); defer timer.Stop(); <-timer.C; <-c.slots }()
		started := time.Now()
		_, _, err := c.Detail(ctx, "2")
		if !errors.Is(err, sourcegate.ErrSourcePaused) || time.Since(started) != 30*time.Millisecond {
			t.Fatalf("error = %v, elapsed = %v", err, time.Since(started))
		}
	})
}

func TestPause_OwnerRetriesAndStops(t *testing.T) {
	for _, succeed := range []bool{true, false} {
		t.Run(map[bool]string{true: "persists", false: "cancel"}[succeed], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				g := &failingPauseGate{Memory: sourcegate.NewMemory(), called: make(chan struct{}, 20), allow: make(chan struct{})}
				var requests atomic.Int32
				c := NewWithBase("http://127.0.0.1", "test", WithTransport(forbiddenTransport(&requests)), WithLimits(float64(rate.Inf), 1), WithBackoffs(time.Millisecond), WithGate(g))
				ctx, cancel := context.WithCancel(sourcegate.WithRunID(context.Background(), 1))
				defer cancel()
				ctx = sourcegate.WithActivity(ctx)
				result := make(chan error, 1)
				go func() { _, _, err := c.Detail(ctx, "1"); result <- err }()
				synctest.Wait()
				select {
				case err := <-result:
					if !errors.Is(err, sourcegate.ErrSourcePaused) {
						t.Fatalf("error = %v", err)
					}
				default:
					t.Fatal("403 request waited for pause")
				}
				select {
				case <-g.called:
				default:
					t.Fatal("owner did not try pause")
				}
				if succeed {
					close(g.allow)
					timer := time.NewTimer(5 * time.Millisecond)
					defer timer.Stop()
					select {
					case <-g.called:
					case <-timer.C:
						t.Fatal("owner did not retry")
					}
					synctest.Wait()
					if err := g.Admit(ctx, 1, "vbpl"); !errors.Is(err, sourcegate.ErrSourcePaused) {
						t.Fatalf("admit: %v", err)
					}
					if _, _, err := c.Detail(ctx, "2"); !errors.Is(err, sourcegate.ErrSourcePaused) || requests.Load() != 1 {
						t.Fatalf("later error = %v, requests = %d", err, requests.Load())
					}
				} else {
					before := g.writes.Load()
					cancel()
					synctest.Wait()
					timer := time.NewTimer(10 * time.Millisecond)
					<-timer.C
					synctest.Wait()
					if g.writes.Load() != before {
						t.Fatal("owner continued after cancellation")
					}
				}
			})
		})
	}
}
