package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
	"golang.org/x/time/rate"
)

type lifetimeGate struct {
	*sourcegate.Memory
	started, release, stopped chan struct{}
}

func (g *lifetimeGate) Pause(ctx context.Context, id int64, source, reason, ref string) error {
	close(g.started)
	defer close(g.stopped)
	select {
	case <-g.release:
		return g.Memory.Pause(ctx, id, source, reason, ref)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestFetch_PauseOwnerOutlivesRequestGroup(t *testing.T) {
	for _, persist := range []bool{true, false} {
		t.Run(map[bool]string{true: "persist", false: "cancel"}[persist], func(t *testing.T) {
			g := &lifetimeGate{Memory: sourcegate.NewMemory(), started: make(chan struct{}), release: make(chan struct{}), stopped: make(chan struct{})}
			var complete atomic.Int32
			others := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/diagram") {
					<-others
					w.WriteHeader(http.StatusForbidden)
					return
				}
				if strings.Contains(r.URL.Path, "/doc/66518") {
					_, _ = w.Write([]byte(`{"success":true,"data":{"id":"66518"}}`))
				} else {
					_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
				}
				if complete.Add(1) == 1 {
					close(others)
				}
			}))
			defer server.Close()
			client := vbpl.NewWithBase(server.URL, "test", vbpl.WithLimits(float64(rate.Inf), 8), vbpl.WithBackoffs(time.Millisecond), vbpl.WithGate(g))
			ctx, cancel := context.WithCancel(sourcegate.WithRunID(context.Background(), 1))
			defer cancel()
			ctx = sourcegate.WithActivity(ctx)
			_, err := New(client).Process(ctx, stage.Env{}, stage.Run{}, stage.Item{Key: "vn:vbpl:66518"})
			if !errors.Is(err, sourcegate.ErrSourcePaused) {
				t.Fatalf("process error = %v", err)
			}
			select {
			case <-g.started:
			case <-time.After(50 * time.Millisecond):
				t.Fatal("pause owner did not start")
			}
			if persist {
				close(g.release)
			} else {
				cancel()
			}
			select {
			case <-g.stopped:
			case <-time.After(50 * time.Millisecond):
				t.Fatal("pause owner did not stop")
			}
			if persist {
				if err := g.Admit(ctx, 1, "vbpl"); !errors.Is(err, sourcegate.ErrSourcePaused) {
					t.Fatalf("pause was not persisted: %v", err)
				}
				if _, _, err := client.Detail(ctx, "66518"); !errors.Is(err, sourcegate.ErrSourcePaused) {
					t.Fatalf("later request = %v", err)
				}
			}
		})
	}
}
