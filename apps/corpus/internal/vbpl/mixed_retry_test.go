package vbpl

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

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
		{"rate limit then 5xx", []int{429, 500}, []time.Duration{time.Millisecond}, 40 * time.Millisecond, "http_429", 429},
		{"5xx then sequence deadline", []int{503}, []time.Duration{20 * time.Millisecond}, 5 * time.Millisecond, "http_5xx", 503},
		{"rate limit then sequence deadline", []int{429}, []time.Duration{20 * time.Millisecond}, 5 * time.Millisecond, "http_429", 429},
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
			c := NewWithBase(s.URL, "test", WithLimits(float64(rate.Inf), 8), WithBackoffs(tc.backoffs...), WithTestTimeouts(10*time.Millisecond, tc.budget))
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
			c := NewWithBase(s.URL, "test", WithLimits(float64(rate.Inf), 8), WithBackoffs(time.Millisecond), WithTestTimeouts(10*time.Millisecond, 50*time.Millisecond))
			_, err := c.Diagram(context.Background(), "1")
			if tc.wantNotFound && !errors.Is(err, ErrNotFound) || !tc.wantNotFound && err != nil {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestAPI_ParentCancellationOverridesPriorStatus(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		<-r.Context().Done()
	}))
	defer s.Close()
	c := NewWithBase(s.URL, "test", WithLimits(float64(rate.Inf), 8), WithBackoffs(time.Millisecond), WithTestTimeouts(50*time.Millisecond, 80*time.Millisecond))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := c.Diagram(ctx, "1")
	if !errors.Is(err, context.DeadlineExceeded) || calls.Load() != 2 {
		t.Fatalf("error = %v, calls = %d", err, calls.Load())
	}
}
