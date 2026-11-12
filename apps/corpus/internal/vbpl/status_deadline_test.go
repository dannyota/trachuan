package vbpl

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
)

type stalledPrefix struct {
	ctx    context.Context
	head   []byte
	closed chan struct{}
	once   sync.Once
}
type statusTransport func(*http.Request) (*http.Response, error)

func (f statusTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func (b *stalledPrefix) Read(p []byte) (int, error) {
	if len(b.head) > 0 {
		n := copy(p, b.head)
		b.head = b.head[n:]
		return n, nil
	}
	select {
	case <-b.ctx.Done():
		return 0, b.ctx.Err()
	case <-b.closed:
		return 0, io.ErrClosedPipe
	}
}
func (b *stalledPrefix) Close() error { b.once.Do(func() { close(b.closed) }); return nil }

type deadlineGate struct {
	*sourcegate.Memory
	writes    int
	remaining time.Duration
}

func (g *deadlineGate) Pause(ctx context.Context, id int64, source, reason, evidence string) error {
	g.writes++
	deadline, _ := ctx.Deadline()
	g.remaining = time.Until(deadline)
	return g.Memory.Pause(ctx, id, source, reason, evidence)
}

func TestStatus_StalledPrefixKeepsHeadersAndPause(t *testing.T) {
	for _, tc := range []struct {
		name          string
		status, calls int
		head          string
		pause         bool
	}{
		{"429", 429, 4, "", false}, {"403", 403, 1, "", true}, {"challenge", 503, 1, "<html>captcha</html>", true}, {"404", 404, 1, "", false}, {"503", 503, 2, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				g := &deadlineGate{Memory: sourcegate.NewMemory()}
				calls := 0
				tr := statusTransport(func(r *http.Request) (*http.Response, error) {
					calls++
					return &http.Response{StatusCode: tc.status, Header: http.Header{"Retry-After": []string{"3"}}, Body: &stalledPrefix{ctx: r.Context(), head: []byte(tc.head), closed: make(chan struct{})}}, nil
				})
				c := NewWithBase("http://127.0.0.1", "test", WithGate(g), WithBackoffs(0, 0, 0), WithLimits(100000, 16))
				c.http.Transport = tr
				ctx := sourcegate.WithActivity(sourcegate.WithRunID(context.Background(), 1))
				start := time.Now()
				_, _, err := c.Detail(ctx, "1")
				synctest.Wait()
				if calls != tc.calls || time.Since(start) > 30*time.Second {
					t.Fatalf("calls=%d elapsed=%s error=%v", calls, time.Since(start), err)
				}
				if tc.pause {
					if !errors.Is(err, sourcegate.ErrSourcePaused) || g.writes != 1 || g.remaining <= 9*time.Second || g.remaining > 10*time.Second {
						t.Fatalf("writes=%d pause deadline=%s error=%v", g.writes, g.remaining, err)
					}
					if err = g.Admit(ctx, 1, "vbpl"); !errors.Is(err, sourcegate.ErrSourcePaused) {
						t.Fatalf("admission=%v", err)
					}
					_, _, err = c.Detail(ctx, "2")
					if !errors.Is(err, sourcegate.ErrSourcePaused) || calls != 1 {
						t.Fatalf("later calls=%d error=%v", calls, err)
					}
				} else {
					var f *Failure
					if !errors.As(err, &f) || f.HTTPStatus != tc.status {
						t.Fatalf("failure=%+v error=%v", f, err)
					}
				}
			})
		})
	}
}

func TestStatus_429SurvivesCallerCapWithinActivity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := NewWithBase("http://127.0.0.1", "test", WithBackoffs(0, 0, 0))
		calls := 0
		c.http.Transport = statusTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{"3"}}, Body: &stalledPrefix{ctx: r.Context(), closed: make(chan struct{})}}, nil
		})
		activity := sourcegate.WithActivity(context.Background())
		request, cancel := context.WithTimeout(activity, time.Second)
		defer cancel()
		_, _, err := c.Detail(request, "1")
		var f *Failure
		if calls != 1 || !errors.As(err, &f) || f.Cause != "http_429" {
			t.Fatalf("calls=%d failure=%+v error=%v", calls, f, err)
		}
	})
}
