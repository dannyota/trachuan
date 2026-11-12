//go:build integration

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
		cause    string
		status   int
		calls    int32
	}{
		{"429 then 5xx", []int{429, 500}, "http_5xx", 500, 3},
		{"5xx then 429", []int{500, 429}, "http_429", 429, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				i := int(calls.Add(1)) - 1
				w.WriteHeader(tc.statuses[min(i, len(tc.statuses)-1)])
			}))
			defer s.Close()
			c := NewWithBase(s.URL, "test", WithLimits(float64(rate.Inf), 8), WithBackoffs(0, 0, 0))
			_, err := c.Diagram(context.Background(), "1")
			var f *Failure
			if !errors.As(err, &f) || f.Cause != tc.cause || f.HTTPStatus != tc.status || calls.Load() != tc.calls {
				t.Fatalf("failure=%+v calls=%d error=%v", f, calls.Load(), err)
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
