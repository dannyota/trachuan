package release

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type waitKey struct{}

type readerWait struct {
	mu      sync.Mutex
	buckets [13]int
	count   int
	max     time.Duration
	logger  *slog.Logger
	release string
}

func (r *Resolver) readerWait(release string) *readerWait {
	return newReaderWait(r.Logger, release)
}

func newReaderWait(logger *slog.Logger, release string) *readerWait {
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	return &readerWait{logger: logger, release: release}
}

func (w *readerWait) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	return ctx
}
func (w *readerWait) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
func (w *readerWait) TraceAcquireStart(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireStartData) context.Context {
	return context.WithValue(ctx, waitKey{}, time.Now())
}
func (w *readerWait) TraceAcquireEnd(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireEndData) {
	if start, ok := ctx.Value(waitKey{}).(time.Time); ok {
		w.record(time.Since(start))
	}
}
func (w *readerWait) record(d time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	bounds := [...]time.Duration{1, 2, 4, 8, 16, 32, 50, 64, 128, 256, 512, 1024}
	i := 0
	for i < len(bounds) && d > bounds[i]*time.Millisecond {
		i++
	}
	w.buckets[i]++
	w.count++
	if d > w.max {
		w.max = d
	}
}
func (w *readerWait) flush() {
	w.mu.Lock()
	buckets, count, maximum := w.buckets, w.count, w.max
	w.count = 0
	w.max = 0
	w.buckets = [13]int{}
	w.mu.Unlock()
	if count == 0 {
		return
	}
	percentile := func(n int) float64 {
		total := 0
		bounds := [...]float64{1, 2, 4, 8, 16, 32, 50, 64, 128, 256, 512, 1024}
		for i, value := range buckets {
			total += value
			if total >= n {
				if i == len(bounds) {
					return float64(maximum) / float64(time.Millisecond)
				}
				return bounds[i]
			}
		}
		return float64(maximum) / float64(time.Millisecond)
	}
	w.logger.Info("reader pool wait", "release", w.release, "count", count, "p50_ms", percentile((count+1)/2), "p99_ms", percentile((99*count+99)/100), "max_ms", float64(maximum)/float64(time.Millisecond), "bucket_bounds_ms", []int{1, 2, 4, 8, 16, 32, 50, 64, 128, 256, 512, 1024}, "bucket_counts", buckets)
}
func (w *readerWait) run(stop <-chan struct{}) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			w.flush()
		case <-stop:
			w.flush()
			return
		}
	}
}
