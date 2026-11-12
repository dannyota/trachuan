package chinhphu

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
)

func TestSourceRule_StatusTable(t *testing.T) {
	for _, file := range []bool{false, true} {
		for _, status := range []int{403, 404, 418, 429, 500, 504} {
			t.Run(fmt.Sprintf("%t/%d", file, status), func(t *testing.T) {
				calls := 0
				var waits []time.Duration
				c, base := clientFor(t, func(w http.ResponseWriter, _ *http.Request) { calls++; w.WriteHeader(status) }, WithBackoffs(2*time.Second, 8*time.Second, 30*time.Second), WithClock(time.Now, func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }))
				var err error
				if file {
					_, err = c.Download(context.Background(), base+"/a", &bytes.Buffer{}, 1000)
				} else {
					_, _, err = c.fetchPage(context.Background(), http.MethodGet, base, nil)
				}
				want := 1
				if status >= 500 {
					want = 2
				}
				if status == 429 {
					want = 4
				}
				if err == nil || calls != want {
					t.Fatalf("calls=%d want=%d waits=%v error=%v", calls, want, waits, err)
				}
				if status == 403 && !errors.Is(err, sourcegate.ErrSourcePaused) {
					t.Fatal(err)
				}
				if status == 404 && !errors.Is(err, ErrNotFound) {
					t.Fatal(err)
				}
			})
		}
	}
}
func TestSourceRule_429WaitCap(t *testing.T) {
	for _, after := range []string{"50", "100", "201", "601"} {
		t.Run(after, func(t *testing.T) {
			calls := 0
			c, base := clientFor(t, func(w http.ResponseWriter, _ *http.Request) {
				calls++
				w.Header().Set("Retry-After", after)
				w.WriteHeader(429)
			}, WithBackoffs(2*time.Second, 8*time.Second, 30*time.Second), WithClock(time.Now, func(context.Context, time.Duration) error { return nil }))
			_, _, err := c.fetchPage(context.Background(), http.MethodGet, base, nil)
			want := map[string]int{"50": 4, "100": 3, "201": 1, "601": 1}[after]
			var failure *Failure
			if !errors.As(err, &failure) || calls != want || failure.Cause == "timeout" || errors.Is(err, ErrDeferred) != (after == "601") {
				t.Fatalf("calls=%d failure=%+v error=%v", calls, failure, err)
			}
		})
	}
}

func TestDownload_RetryStartsAtFirstByte(t *testing.T) {
	calls := 0
	c, base := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Range") != "" {
			t.Error("ranged resume")
		}
		w.Header().Set("Content-Type", "application/pdf")
		if calls == 1 {
			w.Header().Set("Content-Length", "100")
			_, _ = w.Write([]byte("%PDF-partial"))
			return
		}
		_, _ = w.Write([]byte("%PDF-complete"))
	}, WithBackoffs(0))
	var dst bytes.Buffer
	info, err := c.Download(context.Background(), base+"/a", &dst, 1000)
	if err != nil || calls != 2 || dst.String() != "%PDF-complete" || info.Size != 13 {
		t.Fatalf("bytes=%s calls=%d info=%+v err=%v", dst.String(), calls, info, err)
	}
}

type brokenGate struct{}

func (brokenGate) Admit(context.Context, int64, string) error {
	return fmt.Errorf("workshop unavailable")
}
func (brokenGate) Pause(context.Context, int64, string, string, string) error {
	return fmt.Errorf("workshop unavailable")
}
func TestSourceRule_GateFailureIsOurError(t *testing.T) {
	c, base := clientFor(t, func(http.ResponseWriter, *http.Request) { t.Error("request passed broken admission") }, WithGate(brokenGate{}))
	_, _, err := c.fetchPage(sourcegate.WithRunID(context.Background(), 1), http.MethodGet, base, nil)
	var failure *Failure
	if err == nil || errors.As(err, &failure) {
		t.Fatalf("own failure classified as source: %v", err)
	}
}

type brokenBody struct{}

func (brokenBody) Read([]byte) (int, error) { return 0, errors.New("broken transport body") }
func TestSourceRule_BrokenBodyRetriesOnce(t *testing.T) {
	calls := 0
	tr := roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/html"}}, Body: io.NopCloser(brokenBody{})}, nil
	})
	c := New(WithBaseURLs("http://127.0.0.1", "http://127.0.0.1"), WithTransport(tr), WithBackoffs(0, 0, 0), WithLimits(Limits{RequestsPerSecond: 100000}))
	_, _, err := c.fetchPage(context.Background(), http.MethodGet, "http://127.0.0.1", nil)
	var failure *Failure
	if !errors.As(err, &failure) || calls != 2 {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}

func TestSourceRule_Broken429BodyKeepsLoadSignal(t *testing.T) {
	for _, file := range []bool{false, true} {
		t.Run(fmt.Sprint(file), func(t *testing.T) {
			calls := 0
			tr := roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: 429, Header: http.Header{}, Body: io.NopCloser(brokenBody{})}, nil
			})
			c := New(WithBaseURLs("http://127.0.0.1", "http://127.0.0.1"), WithTransport(tr), WithBackoffs(0, 0, 0), WithLimits(Limits{RequestsPerSecond: 100000}))
			var err error
			if file {
				_, err = c.Download(context.Background(), "http://127.0.0.1/a", io.Discard, 100)
			} else {
				_, _, err = c.fetchPage(context.Background(), http.MethodGet, "http://127.0.0.1", nil)
			}
			var f *Failure
			if calls != 4 || !errors.As(err, &f) || f.Cause != "http_429" || f.HTTPStatus != 429 {
				t.Fatalf("calls=%d failure=%+v error=%v", calls, f, err)
			}
		})
	}
}
func TestSourceRule_ChallengeOnAnyStatusPauses(t *testing.T) {
	for _, status := range []int{200, 206, 302, 404, 429, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			c, base := clientFor(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`<html><div>captcha challenge-platform</div></html>`))
			}, WithBackoffs(0, 0, 0))
			_, _, err := c.fetchPage(context.Background(), http.MethodGet, base, nil)
			if !errors.Is(err, sourcegate.ErrSourcePaused) {
				t.Fatal(err)
			}
		})
	}
}

func TestSourceRule_429BodyLimitKeepsLoadSignal(t *testing.T) {
	for _, file := range []bool{false, true} {
		t.Run(fmt.Sprint(file), func(t *testing.T) {
			calls := 0
			tr := roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: 429, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", 100)))}, nil
			})
			c := New(WithBaseURLs("http://127.0.0.1", "http://127.0.0.1"), WithTransport(tr), WithBackoffs(0, 0, 0), WithLimits(Limits{RequestsPerSecond: 100000, HTMLBytes: 10, FileBytes: 10}))
			var err error
			if file {
				_, err = c.Download(context.Background(), "http://127.0.0.1/a", io.Discard, 10)
			} else {
				_, _, err = c.fetchPage(context.Background(), http.MethodGet, "http://127.0.0.1", nil)
			}
			var failure *Failure
			if calls != 4 || !errors.As(err, &failure) || failure.Cause != "http_429" {
				t.Fatalf("calls=%d failure=%+v error=%v", calls, failure, err)
			}
		})
	}
}
