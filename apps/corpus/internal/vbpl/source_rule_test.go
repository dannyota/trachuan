package vbpl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
)

func TestSourceRule_OneRetry(t *testing.T) {
	for _, status := range []int{500, 502, 504, 200, 418} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(status)
				_, _ = w.Write([]byte("unavailable"))
			}))
			defer srv.Close()
			c := NewWithBase(srv.URL, "test", WithLimits(100000, 8), WithBackoffs(0, 0, 0))
			_, err := c.Diagram(context.Background(), "1")
			want := int32(2)
			if status == 418 {
				want = 1
			}
			if err == nil || calls.Load() != want {
				t.Fatalf("calls=%d want=%d error=%v", calls.Load(), want, err)
			}
		})
	}
}

func TestDownloadDeadline_Size(t *testing.T) {
	c := NewWithBase("http://127.0.0.1", "test")
	if c.fileTimeout != 30*time.Second {
		t.Fatal(c.fileTimeout)
	}
	// The request transport test below will pin the deadline without wall-clock waits.
}

func TestSourceRule_429UsesRetryAfter(t *testing.T) {
	for _, after := range []string{"50", "100", "201", "601"} {
		t.Run(after, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Retry-After", after)
				w.WriteHeader(429)
			}))
			defer srv.Close()
			var waits []time.Duration
			c := NewWithBase(srv.URL, "test", WithLimits(100000, 8), WithClock(time.Now, func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }))
			_, err := c.Diagram(context.Background(), "1")
			want := map[string]int32{"50": 4, "100": 3, "201": 1, "601": 1}[after]
			if err == nil || calls.Load() != want {
				t.Fatalf("calls=%d waits=%v error=%v", calls.Load(), waits, err)
			}
		})
	}
}

type deadlineRoundTrip func(*http.Request) (*http.Response, error)

func (f deadlineRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestDownloadDeadline_TransportSeesSizeFormula(t *testing.T) {
	c := NewWithBase("http://127.0.0.1", "test", WithTestHTTP(), WithBackoffs())
	c.fileHTTP.Transport = deadlineRoundTrip(func(r *http.Request) (*http.Response, error) {
		deadline, ok := r.Context().Deadline()
		remaining := time.Until(deadline)
		if !ok || remaining < 109*time.Second || remaining > 110*time.Second {
			t.Errorf("deadline %s", remaining)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("x")), Header: http.Header{}}, nil
	})
	_, _ = c.Download(context.Background(), "1", File{URL: "http://127.0.0.1/file", Size: 20_000_000}, t.TempDir(), 50_000_000)
}

func TestSourceRule_ChallengePauses(t *testing.T) {
	for _, file := range []bool{false, true} {
		t.Run(fmt.Sprint(file), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				_, _ = io.WriteString(w, `<html><title>Just a moment</title><div id="cf-chl-widget">Verify you are human</div></html>`)
			}))
			defer srv.Close()
			c := NewWithBase(srv.URL, "test", WithTestHTTP(), WithBackoffs(0))
			var err error
			if file {
				_, err = c.Download(context.Background(), "1", File{Name: "a", URL: srv.URL}, t.TempDir(), 1000)
			} else {
				_, err = c.Diagram(context.Background(), "1")
			}
			if !errors.Is(err, sourcegate.ErrSourcePaused) {
				t.Fatal(err)
			}
		})
	}
}

func TestDownload_RetryStartsAtFirstByte(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Range") != "" {
			t.Error("ranged resume")
		}
		if calls == 1 {
			w.Header().Set("Content-Length", "100")
			_, _ = w.Write([]byte("partial"))
			return
		}
		_, _ = w.Write([]byte("complete"))
	}))
	defer srv.Close()
	c := NewWithBase(srv.URL, "test", WithTestHTTP(), WithBackoffs(0))
	d, err := c.Download(context.Background(), "1", File{URL: srv.URL, Size: 8}, t.TempDir(), 1000)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(d.Path)
	if err != nil || calls != 2 || string(body) != "complete" {
		t.Fatalf("calls=%d bytes=%s err=%v", calls, body, err)
	}
}

type throughputBody struct {
	remaining     int64
	now, deadline time.Time
}

func (b *throughputBody) Read(p []byte) (int, error) {
	if b.remaining == 0 {
		return 0, io.EOF
	}
	n := min(int64(len(p)), b.remaining)
	b.now = b.now.Add(time.Duration(n) * time.Second / 250_000)
	if b.now.After(b.deadline) {
		return 0, context.DeadlineExceeded
	}
	for i := range p[:n] {
		p[i] = 'x'
	}
	b.remaining -= n
	return int(n), nil
}
func TestDownload_TwentyMBAtBudgetedThroughput(t *testing.T) {
	c := NewWithBase("http://127.0.0.1", "test", WithTestHTTP(), WithBackoffs())
	c.fileHTTP.Transport = deadlineRoundTrip(func(r *http.Request) (*http.Response, error) {
		deadline, _ := r.Context().Deadline()
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(&throughputBody{remaining: 20_000_000, now: time.Now(), deadline: deadline})}, nil
	})
	d, err := c.Download(context.Background(), "1", File{URL: "http://127.0.0.1/a", Size: 20_000_000}, t.TempDir(), 50_000_000)
	if err != nil || d.Size != 20_000_000 {
		t.Fatalf("size=%d error=%v", d.Size, err)
	}
}

type brokenBody struct{}

func (brokenBody) Read([]byte) (int, error) { return 0, errors.New("broken transport body") }
func TestSourceRule_BrokenBodyRetriesOnce(t *testing.T) {
	c := NewWithBase("http://127.0.0.1", "test", WithTestHTTP(), WithBackoffs(0, 0, 0))
	calls := 0
	c.fileHTTP.Transport = deadlineRoundTrip(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(brokenBody{})}, nil
	})
	_, err := c.Download(context.Background(), "1", File{URL: "http://127.0.0.1/a"}, t.TempDir(), 100)
	var failure *Failure
	if !errors.As(err, &failure) || calls != 2 {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}
