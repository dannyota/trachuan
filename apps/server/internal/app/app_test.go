package app

import (
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
