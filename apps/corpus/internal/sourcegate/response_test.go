package sourcegate

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestResponsePrefix_Deadline(t *testing.T) {
	for _, limit := range []time.Duration{time.Minute, 2 * time.Second} {
		t.Run(limit.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), limit)
				defer cancel()
				reader, writer := io.Pipe()
				defer func() { _ = writer.Close() }()
				go func() { _, _ = writer.Write([]byte("partial")) }()
				start := time.Now()
				prefix, replay := ResponsePrefix(ctx, reader, reader, 503, 200, nil)
				data, err := io.ReadAll(replay)
				if time.Since(start) != min(limit, 5*time.Second) || string(prefix) != "partial" || string(data) != "partial" || !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("elapsed=%s prefix=%q replay=%q error=%v", time.Since(start), prefix, data, err)
				}
			})
		})
	}
}
func TestResponsePrefix_ReplaysCompleteBody(t *testing.T) {
	for _, body := range []string{"", "small", strings.Repeat("x", 5000)} {
		reader := io.NopCloser(strings.NewReader(body))
		_, replay := ResponsePrefix(context.Background(), reader, reader, 200, 200, nil)
		data, err := io.ReadAll(replay)
		if err != nil || string(data) != body {
			t.Fatalf("size=%d read=%d error=%v", len(body), len(data), err)
		}
	}
}
