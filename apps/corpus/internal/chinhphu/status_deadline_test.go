package chinhphu

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

func TestStatus_Stalled429KeepsLoadAtPageCap(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		tr := roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{"3"}}, Body: &stalledPrefix{ctx: r.Context(), closed: make(chan struct{})}}, nil
		})
		c := New(WithBaseURLs("https://test.invalid", "https://test.invalid"), WithTransport(tr), WithLimits(Limits{RequestsPerSecond: 100000, Burst: 10, ItemTimeout: 29 * time.Second}), WithBackoffs(0, 0, 0))
		ctx := sourcegate.WithActivity(context.Background())
		start := time.Now()
		_, _, err := c.Detail(ctx, 1)
		var f *Failure
		if calls != 4 || !errors.As(err, &f) || f.Cause != "http_429" || time.Since(start) > 29*time.Second {
			t.Fatalf("calls=%d elapsed=%s failure=%+v error=%v", calls, time.Since(start), f, err)
		}
	})
}

func TestStatus_StalledPrefixKeepsStatusAndPause(t *testing.T) {
	for _, tc := range []struct {
		name          string
		status, calls int
		head          string
		pause         bool
	}{
		{"403", 403, 1, "", true}, {"challenge", 503, 1, "<html>captcha challenge-platform</html>", true}, {"404", 404, 1, "", false}, {"503", 503, 2, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				g := &deadlineGate{Memory: sourcegate.NewMemory()}
				calls := 0
				tr := roundTripFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					return &http.Response{StatusCode: tc.status, Header: http.Header{}, Body: &stalledPrefix{ctx: r.Context(), head: []byte(tc.head), closed: make(chan struct{})}}, nil
				})
				c := New(WithBaseURLs("https://test.invalid", "https://test.invalid"), WithTransport(tr), WithGate(g), WithLimits(Limits{RequestsPerSecond: 100000, Burst: 10, PageTimeout: 2 * time.Second}), WithBackoffs(0))
				ctx := sourcegate.WithActivity(sourcegate.WithRunID(context.Background(), 1))
				_, _, err := c.Detail(ctx, 1)
				synctest.Wait()
				if calls != tc.calls {
					t.Fatalf("calls=%d want=%d error=%v", calls, tc.calls, err)
				}
				if tc.pause {
					if !errors.Is(err, sourcegate.ErrSourcePaused) || g.writes != 1 || g.remaining <= 9*time.Second || g.remaining > 10*time.Second {
						t.Fatalf("writes=%d pause deadline=%s error=%v", g.writes, g.remaining, err)
					}
					if err = g.Admit(ctx, 1, "chinhphu"); !errors.Is(err, sourcegate.ErrSourcePaused) {
						t.Fatalf("admission=%v", err)
					}
					_, _, err = c.Detail(ctx, 2)
					if !errors.Is(err, sourcegate.ErrSourcePaused) || calls != 1 {
						t.Fatalf("later calls=%d error=%v", calls, err)
					}
				} else if tc.status == 404 {
					if !errors.Is(err, ErrNotFound) {
						t.Fatal(err)
					}
				} else {
					var f *Failure
					if !errors.As(err, &f) || f.Cause != "http_5xx" {
						t.Fatal(err)
					}
				}
			})
		})
	}
}
