package chinhphu

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
)

func delayedBody(ctx context.Context, body string) io.ReadCloser {
	reader, writer := io.Pipe()
	go func() {
		defer func() { _ = writer.Close() }()
		timer := time.NewTimer(6 * time.Second)
		defer timer.Stop()
		select {
		case <-timer.C:
			_, _ = io.WriteString(writer, body)
		case <-ctx.Done():
			_ = writer.CloseWithError(ctx.Err())
		}
	}()
	return reader
}
func TestSuccessStatus_DelayedBodiesKeepRequestDeadline(t *testing.T) {
	for _, pdf := range []bool{false, true} {
		t.Run(map[bool]string{false: "page", true: "PDF"}[pdf], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				calls := 0
				tr := roundTripFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					body, media := `<div id="ctrl_190596_91_Content"><table><tr><td>Số ký hiệu</td><td>42/2026/QD</td></tr></table></div>`, "text/html"
					if pdf {
						body, media = "%PDF-delayed", "application/pdf"
					}
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{media}}, Body: delayedBody(r.Context(), body)}, nil
				})
				c := New(WithBaseURLs("https://test.invalid", "https://test.invalid"), WithTransport(tr), WithBackoffs(0), WithLimits(Limits{RequestsPerSecond: 100000, PageTimeout: 10 * time.Second, FileTimeout: 10 * time.Second}))
				start := time.Now()
				var err error
				if pdf {
					var info FileInfo
					info, err = c.Download(context.Background(), "https://test.invalid/a.pdf", io.Discard, 100)
					if err == nil && info.Size != 12 {
						t.Fatalf("info=%+v", info)
					}
				} else {
					_, _, err = c.Detail(context.Background(), 1)
				}
				if calls != 1 || err != nil || time.Since(start) != 6*time.Second {
					t.Fatalf("calls=%d elapsed=%s error=%v", calls, time.Since(start), err)
				}
			})
		})
	}
}
func TestSuccessStatus_ChallengeDetectedAsBytesArrive(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := New(WithBaseURLs("https://test.invalid", "https://test.invalid"), WithBackoffs(), WithTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: &stalledPrefix{ctx: r.Context(), head: []byte("<html>captcha challenge-platform</html>"), closed: make(chan struct{})}}, nil
		})))
		start := time.Now()
		_, _, err := c.Detail(sourcegate.WithActivity(context.Background()), 1)
		if !errors.Is(err, sourcegate.ErrSourcePaused) || time.Since(start) != 0 {
			t.Fatalf("elapsed=%s error=%v", time.Since(start), err)
		}
	})
}
func TestUnexpected206_KeepsStatusAtItemCap(t *testing.T) {
	for _, pdf := range []bool{false, true} {
		t.Run(map[bool]string{false: "page", true: "PDF"}[pdf], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				calls := 0
				c := New(WithBaseURLs("https://test.invalid", "https://test.invalid"), WithLimits(Limits{ItemTimeout: time.Second}), WithBackoffs(0, 0, 0), WithTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					return &http.Response{StatusCode: 206, Header: http.Header{}, Body: &stalledPrefix{ctx: r.Context(), closed: make(chan struct{})}}, nil
				})))
				var err error
				if pdf {
					ctx, cancel := context.WithTimeout(sourcegate.WithActivity(context.Background()), time.Second)
					defer cancel()
					_, err = c.Download(ctx, "https://test.invalid/a.pdf", io.Discard, 100)
				} else {
					_, _, err = c.Detail(context.Background(), 1)
				}
				var f *Failure
				if calls != 1 || !errors.As(err, &f) || f.HTTPStatus != 206 || f.Cause != "unknown" {
					t.Fatalf("calls=%d failure=%+v error=%v", calls, f, err)
				}
			})
		})
	}
}
