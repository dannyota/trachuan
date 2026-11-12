//go:build integration

package vbpl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"golang.org/x/time/rate"
)

func TestAPI_MixedRetryStatuses(t *testing.T) {
	for _, tc := range []struct {
		name     string
		statuses []int
		backoffs []time.Duration
		budget   time.Duration
		cause    string
		status   int
	}{
		{"rate limit then 5xx", []int{429, 500}, []time.Duration{time.Millisecond}, 5 * time.Second, "http_429", 429},
		{"5xx then sequence deadline", []int{503}, []time.Duration{5 * time.Second}, 500 * time.Millisecond, "http_5xx", 503},
		{"rate limit then sequence deadline", []int{429}, []time.Duration{5 * time.Second}, 500 * time.Millisecond, "http_429", 429},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				i := int(calls.Add(1)) - 1
				if i >= len(tc.statuses) {
					t.Errorf("unexpected request %d", i)
					w.WriteHeader(500)
					return
				}
				w.WriteHeader(tc.statuses[i])
			}))
			defer s.Close()
			c := NewWithBase(s.URL, "test", WithLimits(float64(rate.Inf), 8), WithBackoffs(tc.backoffs...), WithTestTimeouts(3*time.Second, tc.budget))
			_, err := c.Diagram(context.Background(), "1")
			var f *Failure
			if !errors.As(err, &f) || f.Origin != "gateway" || f.Cause != tc.cause || f.Endpoint != "doc/diagram" || f.HTTPStatus != tc.status {
				t.Fatalf("failure=%+v err=%v", f, err)
			}
		})
	}
}

func TestAPI_PriorStatusDoesNotOverrideFinalResponse(t *testing.T) {
	for _, tc := range []struct {
		name         string
		secondStatus int
		wantNotFound bool
	}{
		{"successful retry", http.StatusOK, false},
		{"not found", http.StatusNotFound, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if calls.Add(1) == 1 {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.WriteHeader(tc.secondStatus)
				if tc.secondStatus == http.StatusOK {
					_, _ = w.Write([]byte(`{"success":true,"data":{}}`))
				}
			}))
			defer s.Close()
			c := NewWithBase(s.URL, "test", WithLimits(float64(rate.Inf), 8), WithBackoffs(time.Millisecond), WithTestTimeouts(3*time.Second, 5*time.Second))
			_, err := c.Diagram(context.Background(), "1")
			if tc.wantNotFound && !errors.Is(err, ErrNotFound) || !tc.wantNotFound && err != nil {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestAPI_ParentCancellationOverridesPriorStatus(t *testing.T) {
	var calls atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		cancel()
		<-r.Context().Done()
	}))
	defer s.Close()
	c := NewWithBase(s.URL, "test", WithLimits(float64(rate.Inf), 8), WithBackoffs(time.Millisecond), WithTestTimeouts(3*time.Second, 5*time.Second))
	_, err := c.Diagram(ctx, "1")
	if !errors.Is(err, context.Canceled) || calls.Load() != 2 {
		t.Fatalf("error = %v, calls = %d", err, calls.Load())
	}
}

func TestClient_LimitsRate(t *testing.T) {
	b := fixture(t, "detail-67878.json")
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(b) }))
	defer s.Close()
	c := NewWithBase(s.URL, "test", WithLimits(50, 8))
	start := time.Now()
	for i := 0; i < 11; i++ {
		if _, _, err := c.Detail(context.Background(), "67878"); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed < 190*time.Millisecond {
		t.Fatalf("11 requests took %v, want at least 190ms", elapsed)
	}
}

func TestDownload_TimeoutAfterPartialBodyRetriesFresh(t *testing.T) {
	var n atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			_, _ = w.Write([]byte("part"))
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		_, _ = w.Write([]byte("complete"))
	}))
	defer s.Close()
	c := NewWithBase(s.URL, "test", WithTestHTTP(), WithLimits(float64(rate.Inf), 8), WithBackoffs(time.Millisecond, time.Millisecond, time.Millisecond), WithTestTimeouts(500*time.Millisecond, 5*time.Second))
	got, err := c.Download(context.Background(), "1", File{URL: s.URL}, t.TempDir(), 100)
	sum := sha256.Sum256([]byte("complete"))
	if err != nil || got.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestClient_PersistsPauseAfterAttemptDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	gate := &waitingPauseGate{Memory: sourcegate.NewMemory(), started: make(chan struct{}), release: make(chan struct{})}
	c := NewWithBase(server.URL, "test", WithLimits(float64(rate.Inf), 1), WithBackoffs(), WithGate(gate))
	c.timeout = 500 * time.Millisecond
	ctx := sourcegate.WithActivity(sourcegate.WithRunID(context.Background(), 1))
	_, _, err := c.Detail(ctx, "1")
	if !errors.Is(err, sourcegate.ErrSourcePaused) {
		t.Fatalf("error = %v", err)
	}
	<-gate.started
	time.Sleep(500 * time.Millisecond)
	close(gate.release)
	waitForPaused(t, ctx, c, 1)
}
