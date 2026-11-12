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

func TestDeadlines_ClassifiedTimeout(t *testing.T) {
	for _, tc := range []string{"rate", "slot", "gate", "backoff", "download"} {
		t.Run(tc, func(t *testing.T) {
			c := NewWithBase("https://example.invalid", "test", WithLimits(0.01, 1), WithFileInFlight(1), WithBackoffs(30*time.Millisecond))
			c.timeout, c.requestBudget = 5*time.Millisecond, 12*time.Millisecond
			var err error
			switch tc {
			case "rate":
				c.limiter.Allow()
				_, _, err = c.Detail(context.Background(), "1")
			case "backoff":
				c.slots <- struct{}{}
				_, _, err = c.Detail(context.Background(), "1")
			case "slot":
				c.backoffs = nil
				c.slots <- struct{}{}
				_, _, err = c.Detail(context.Background(), "1")
			case "gate":
				c.backoffs = nil
				c.gate = blockedAdmission{sourcegate.NewMemory()}
				_, _, err = c.Detail(sourcegate.WithRunID(context.Background(), 1), "1")
			case "download":
				c.fileSlots <- struct{}{}
				_, err = c.Download(context.Background(), "1", File{URL: "https://example.invalid/file"}, t.TempDir(), 10)
			}
			var failure *Failure
			if !errors.As(err, &failure) || failure.Origin != "transport" || failure.Cause != "timeout" || !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("failure = %+v, error = %v", failure, err)
			}
		})
	}
}

type blockedAdmission struct{ *sourcegate.Memory }

func (blockedAdmission) Admit(ctx context.Context, _ int64, _ string) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestDeadlines_ActivityCancellation(t *testing.T) {
	c := NewWithBase("https://example.invalid", "test", WithBackoffs(30*time.Millisecond))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := c.Detail(ctx, "1")
	var failure *Failure
	if !errors.Is(err, context.Canceled) || errors.As(err, &failure) {
		t.Fatalf("error = %v, want activity cancellation", err)
	}
}

type slowPauseGate struct{ *sourcegate.Memory }

func (g slowPauseGate) Pause(ctx context.Context, id int64, source, reason, evidence string) error {
	select {
	case <-time.After(45 * time.Millisecond):
		return g.Memory.Pause(ctx, id, source, reason, evidence)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestClient_PersistsPauseAfterAttemptDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	gate := slowPauseGate{sourcegate.NewMemory()}
	c := NewWithBase(server.URL, "test", WithLimits(float64(rate.Inf), 1), WithBackoffs(), WithGate(gate))
	c.timeout = 30 * time.Millisecond
	ctx := sourcegate.WithActivity(sourcegate.WithRunID(context.Background(), 1))
	_, _, err := c.Detail(ctx, "1")
	waitForPaused(t, ctx, gate, 1)
	if !errors.Is(err, sourcegate.ErrSourcePaused) || !errors.Is(gate.Admit(ctx, 1, "vbpl"), sourcegate.ErrSourcePaused) {
		t.Fatalf("error = %v, pause not persisted", err)
	}
}

type waitingPauseGate struct {
	*sourcegate.Memory
	started, release chan struct{}
}

func (g *waitingPauseGate) Pause(ctx context.Context, id int64, source, reason, evidence string) error {
	close(g.started)
	select {
	case <-g.release:
		return g.Memory.Pause(ctx, id, source, reason, evidence)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestClient_WaitsForPauseBeforeNextRequest(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	gate := &waitingPauseGate{Memory: sourcegate.NewMemory(), started: make(chan struct{}), release: make(chan struct{})}
	c := NewWithBase(server.URL, "test", WithLimits(float64(rate.Inf), 2), WithBackoffs(time.Millisecond), WithGate(gate))
	ctx := sourcegate.WithActivity(sourcegate.WithRunID(context.Background(), 1))
	errs := make(chan error, 2)
	go func() { _, _, err := c.Detail(ctx, "1"); errs <- err }()
	<-gate.started
	go func() { _, _, err := c.Detail(ctx, "2"); errs <- err }()
	time.Sleep(10 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Errorf("requests while pause pending = %d", got)
	}
	close(gate.release)
	for range 2 {
		if err := <-errs; !errors.Is(err, sourcegate.ErrSourcePaused) {
			t.Errorf("error = %v", err)
		}
	}
}

type retryPauseGate struct {
	*sourcegate.Memory
	writes atomic.Int32
}

func (g *retryPauseGate) Pause(ctx context.Context, id int64, source, reason, evidence string) error {
	if g.writes.Add(1) == 1 {
		return errors.New("pause unavailable")
	}
	return g.Memory.Pause(ctx, id, source, reason, evidence)
}

func TestClient_RetriesFailedPauseBeforeRequest(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.WriteHeader(http.StatusForbidden) }))
	defer server.Close()
	gate := &retryPauseGate{Memory: sourcegate.NewMemory()}
	c := NewWithBase(server.URL, "test", WithLimits(float64(rate.Inf), 2), WithBackoffs(time.Millisecond), WithGate(gate))
	ctx := sourcegate.WithActivity(sourcegate.WithRunID(context.Background(), 1))
	_, _, _ = c.Detail(ctx, "1")
	_, _, err := c.Detail(ctx, "2")
	if !errors.Is(err, sourcegate.ErrSourcePaused) || calls.Load() != 1 || gate.writes.Load() != 2 {
		t.Fatalf("error = %v, requests = %d, pause writes = %d", err, calls.Load(), gate.writes.Load())
	}
}

func TestGateway_DefaultAttemptDeadline(t *testing.T) {
	c := NewWithBase("http://example.invalid", "test")
	if c.timeout != 40*time.Second || c.requestBudget != 200*time.Second {
		t.Fatalf("attempt = %s, sequence = %s; want 40s and 200s", c.timeout, c.requestBudget)
	}
}
