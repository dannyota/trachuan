package vbpl

import (
	"context"
	"errors"
	"time"

	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"golang.org/x/time/rate"
)

func TestAPI_NonJSON200Retryable(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		_, _ = w.Write([]byte("<html>synthetic gateway error</html>"))
	}))
	defer server.Close()
	_, _, err := testClient(server).Detail(context.Background(), "1")
	var failure *Failure
	if !errors.As(err, &failure) || failure.Origin != "gateway" || failure.Cause != "non_json_200" || failure.Endpoint != "doc/detail" || failure.HTTPStatus != 200 || attempts.Load() != 4 {
		t.Fatalf("error = %v, attempts = %d, want classified error and 4 attempts", err, attempts.Load())
	}
}

func TestClient_ForbiddenPausesSource(t *testing.T) {
	var attempts atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { attempts.Add(1); w.WriteHeader(http.StatusForbidden) }))
	defer s.Close()
	gate := sourcegate.NewMemory()
	c := NewWithBase(s.URL, "test", WithLimits(float64(rate.Inf), 8), WithBackoffs(time.Millisecond), WithGate(gate))
	ctx := sourcegate.WithRunID(context.Background(), 7)
	_, _, err := c.Detail(ctx, "1")
	waitForPaused(t, ctx, gate, 7)
	if !errors.Is(err, sourcegate.ErrSourcePaused) || attempts.Load() != 1 || !errors.Is(gate.Admit(ctx, 7, "vbpl"), sourcegate.ErrSourcePaused) {
		t.Fatalf("err = %v, attempts = %d", err, attempts.Load())
	}
}

func waitForPaused(t *testing.T, ctx context.Context, gate sourcegate.Gate, id int64) {
	t.Helper()
	deadline := time.After(90 * time.Millisecond)
	for !errors.Is(gate.Admit(ctx, id, "vbpl"), sourcegate.ErrSourcePaused) {
		select {
		case <-deadline:
			t.Fatal("source pause was not persisted")
		case <-time.After(time.Millisecond):
		}
	}
}

func TestClient_PausedGateSendsNoRequest(t *testing.T) {
	var attempts atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { attempts.Add(1) }))
	defer s.Close()
	gate := sourcegate.NewMemory()
	ctx := sourcegate.WithRunID(context.Background(), 7)
	if err := gate.Pause(ctx, 7, "vbpl", "test", ""); err != nil {
		t.Fatal(err)
	}
	c := NewWithBase(s.URL, "test", WithGate(gate))
	_, _, err := c.Detail(ctx, "1")
	if !errors.Is(err, sourcegate.ErrSourcePaused) || attempts.Load() != 0 {
		t.Fatalf("err = %v, attempts = %d", err, attempts.Load())
	}
}

func TestClient_GateWithoutRunFailsClosed(t *testing.T) {
	var attempts atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { attempts.Add(1) }))
	defer s.Close()
	c := NewWithBase(s.URL, "test", WithGate(sourcegate.NewMemory()))
	_, _, err := c.Detail(context.Background(), "1")
	if err == nil || attempts.Load() != 0 {
		t.Fatalf("err = %v, attempts = %d", err, attempts.Load())
	}
}

func TestClient_RequestSlotWaitUsesRequestTimeout(t *testing.T) {
	c := NewWithBase("https://example.invalid", "test", WithLimits(float64(rate.Inf), 1), WithBackoffs())
	c.timeout = 5 * time.Millisecond
	c.slots <- struct{}{}
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	_, _, err := c.Detail(ctx, "1")
	if !errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		t.Fatalf("error = %v, parent context error = %v; want request timeout before parent", err, ctx.Err())
	}
}

func TestClient_RetrySequenceHasDeadline(t *testing.T) {
	c := NewWithBase("https://example.invalid", "test", WithLimits(float64(rate.Inf), 1), WithBackoffs(30*time.Millisecond))
	c.timeout = 5 * time.Millisecond
	c.requestBudget = 10 * time.Millisecond
	c.slots <- struct{}{}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	_, _, err := c.Detail(ctx, "1")
	if !errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		t.Fatalf("error = %v, parent = %v", err, ctx.Err())
	}
}

func TestAPI_FailureCauses(t *testing.T) {
	for _, tc := range []struct {
		status        int
		cause, origin string
	}{{500, "http_5xx", "gateway"}, {429, "http_429", "gateway"}, {0, "timeout", "transport"}} {
		t.Run(tc.cause, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.status == 0 {
					<-r.Context().Done()
					return
				}
				w.WriteHeader(tc.status)
			}))
			defer s.Close()
			c := testClient(s)
			c.timeout = 5 * time.Millisecond
			_, _, err := c.Detail(context.Background(), "1")
			var f *Failure
			if !errors.As(err, &f) || f.Cause != tc.cause || f.Origin != tc.origin || f.HTTPStatus != tc.status || f.Endpoint != "doc/detail" {
				t.Fatalf("failure = %+v, err = %v", f, err)
			}
		})
	}
}

func TestAPI_InterruptedResponseIsConnectionError(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte(`{"success":true`))
	}))
	defer s.Close()
	_, _, err := testClient(s).Detail(context.Background(), "1")
	var f *Failure
	if !errors.As(err, &f) || f.Origin != "transport" || f.Cause != "connection_error" || f.Endpoint != "doc/detail" || f.HTTPStatus != 200 {
		t.Fatalf("failure = %+v, error = %v", f, err)
	}
}
