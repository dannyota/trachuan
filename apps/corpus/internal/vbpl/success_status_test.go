package vbpl

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
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
	for _, kind := range []string{"gateway", "files", "attachment", "ETag"} {
		t.Run(kind, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				calls := 0
				c := NewWithBase("http://127.0.0.1", "test", WithTestHTTP(), WithTestTimeouts(10*time.Second, 200*time.Second), WithBackoffs(0))
				tr := statusTransport(func(r *http.Request) (*http.Response, error) {
					calls++
					status := 200
					body := `{"success":true,"data":{"id":"1","title":"` + strings.Repeat("x", 4096) + `"}}`
					switch kind {
					case "files":
						body = `{"success":true,"data":[]}`
					case "attachment":
						body = strings.Repeat("x", 4096)
					case "ETag":
						status = 206
						body = "x"
					}
					return &http.Response{StatusCode: status, Header: http.Header{"Etag": []string{`"d41d8cd98f00b204e9800998ecf8427e"`}}, Body: delayedBody(r.Context(), body)}, nil
				})
				c.http.Transport = tr
				c.fileHTTP.Transport = tr
				start := time.Now()
				var err error
				switch kind {
				case "gateway":
					_, _, err = c.Detail(context.Background(), "1")
				case "files":
					_, err = c.Files(context.Background(), "1")
				case "attachment":
					var d Downloaded
					d, err = c.Download(context.Background(), "1", File{Name: "a", URL: "http://127.0.0.1/a", Size: 4096}, t.TempDir(), 5000)
					if err == nil && d.Size != 4096 {
						t.Fatalf("download=%+v", d)
					}
				case "ETag":
					var etag string
					etag, err = c.ETag(context.Background(), "http://127.0.0.1/a")
					if err == nil && etag != "d41d8cd98f00b204e9800998ecf8427e" {
						t.Fatalf("etag=%q", etag)
					}
				}
				if err != nil || calls != 1 || time.Since(start) != 6*time.Second {
					t.Fatalf("calls=%d elapsed=%s error=%v", calls, time.Since(start), err)
				}
			})
		})
	}
}

func TestSuccessStatus_ChallengeDetectedAsBytesArrive(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := NewWithBase("http://127.0.0.1", "test", WithGate(sourcegate.NewMemory()), WithBackoffs())
		c.http.Transport = statusTransport(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: &stalledPrefix{ctx: r.Context(), head: []byte("<html>captcha</html>"), closed: make(chan struct{})}}, nil
		})
		ctx := sourcegate.WithActivity(sourcegate.WithRunID(context.Background(), 1))
		start := time.Now()
		_, _, err := c.Detail(ctx, "1")
		if !errors.Is(err, sourcegate.ErrSourcePaused) || time.Since(start) != 0 {
			t.Fatalf("elapsed=%s error=%v", time.Since(start), err)
		}
	})
}

func TestUnexpected206_KeepsStatusAtCallerCap(t *testing.T) {
	for _, kind := range []string{"gateway", "files", "attachment"} {
		t.Run(kind, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				calls := 0
				c := NewWithBase("http://127.0.0.1", "test", WithTestHTTP(), WithBackoffs(0, 0, 0))
				tr := statusTransport(func(r *http.Request) (*http.Response, error) {
					calls++
					return &http.Response{StatusCode: 206, Header: http.Header{}, Body: &stalledPrefix{ctx: r.Context(), closed: make(chan struct{})}}, nil
				})
				c.http.Transport = tr
				c.fileHTTP.Transport = tr
				ctx, cancel := context.WithTimeout(sourcegate.WithActivity(context.Background()), time.Second)
				defer cancel()
				var err error
				switch kind {
				case "gateway":
					_, _, err = c.Detail(ctx, "1")
				case "files":
					_, err = c.Files(ctx, "1")
				case "attachment":
					_, err = c.Download(ctx, "1", File{Name: "a", URL: "http://127.0.0.1/a", Size: 1}, t.TempDir(), 100)
				}
				var f *Failure
				if calls != 1 || !errors.As(err, &f) || f.HTTPStatus != 206 || f.Cause != "unknown" {
					t.Fatalf("calls=%d failure=%+v error=%v", calls, f, err)
				}
			})
		})
	}
}
