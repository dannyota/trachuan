package app

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRun_ReadyOnlyWithAuthAndActiveRelease(t *testing.T) {
	for _, ok := range []bool{false, true} {
		h := readiness(context.Background(), func(context.Context) error {
			if !ok {
				return errors.New("unavailable")
			}
			return nil
		})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		want := http.StatusServiceUnavailable
		if ok {
			want = http.StatusOK
		}
		if w.Code != want {
			t.Fatalf("ready=%t: %d", ok, w.Code)
		}
	}
}

func TestReadyCheck_AuthAndRelease(t *testing.T) {
	for _, tt := range []struct {
		name                string
		authErr, releaseErr error
		wantCalls           int
		wantOK              bool
	}{
		{name: "ready", wantCalls: 1, wantOK: true},
		{name: "auth unavailable", authErr: errors.New("auth"), wantCalls: 0},
		{name: "release unavailable", releaseErr: errors.New("release"), wantCalls: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			err := readyCheck(context.Background(), func(context.Context) error { return tt.authErr }, func(context.Context) error { calls++; return tt.releaseErr })
			if (err == nil) != tt.wantOK || calls != tt.wantCalls {
				t.Fatalf("err=%v release calls=%d", err, calls)
			}
		})
	}
}

func TestReadiness_CanceledDuringCheck(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered, proceed, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	h := readiness(ctx, func(context.Context) error { close(entered); <-proceed; return nil })
	w := httptest.NewRecorder()
	go func() { h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil)); close(done) }()
	<-entered
	cancel()
	close(proceed)
	<-done
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

type bindFailureServer struct{}

func (bindFailureServer) ListenAndServe() error          { return errors.New("address already in use") }
func (bindFailureServer) Shutdown(context.Context) error { return nil }
func (bindFailureServer) Close() error                   { return nil }

func TestRun_BindFailure(t *testing.T) {
	closed := false
	err := runHTTP(context.Background(), bindFailureServer{}, func() { closed = true })
	if err == nil || !closed {
		t.Fatalf("error=%v resolver closed=%t", err, closed)
	}
	var stderr bytes.Buffer
	if exitCode(err, &stderr) == 0 {
		t.Fatal("bind failure returned success")
	}
}
