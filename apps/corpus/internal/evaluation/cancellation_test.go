package evaluation

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"danny.vn/trachuan/internal/retrieval"
)

type cancellationEngine struct {
	fakeEngine
	target    string
	started   chan struct{}
	cancelled chan struct{}
	release   chan struct{}
	once      sync.Once
	active    atomic.Int32
}

func (e *cancellationEngine) Prepare(ctx context.Context, request retrieval.Request) (retrieval.Prepared, error) {
	if err := ctx.Err(); err != nil {
		return retrieval.Prepared{}, err
	}
	return e.fakeEngine.Prepare(ctx, request)
}

func (e *cancellationEngine) Collect(ctx context.Context, p retrieval.Prepared, _ retrieval.Depths) (retrieval.Arms, error) {
	e.active.Add(1)
	defer e.active.Add(-1)
	if p.Request.Query == e.target {
		e.once.Do(func() { close(e.started) })
		<-ctx.Done()
		close(e.cancelled)
		<-e.release
		return retrieval.Arms{}, fmt.Errorf("collect failed: %w", ctx.Err())
	}
	<-ctx.Done()
	return retrieval.Arms{}, ctx.Err()
}

func TestCasePools_ParentCancellationDuringCollect(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: time.November, Day: 9}
	for _, calibration := range []bool{false, true} {
		t.Run(fmt.Sprintf("calibration=%t", calibration), func(t *testing.T) {
			var got []string
			for _, limit := range []int{1, 3} {
				suite := recordedSuite()
				engine := &cancellationEngine{fakeEngine: fakeEngine{date: date}, target: suite.Legacy[0].Question, started: make(chan struct{}), cancelled: make(chan struct{}), release: make(chan struct{})}
				ctx, cancel := context.WithCancel(context.Background())
				result := make(chan error, 1)
				go func() {
					if calibration {
						_, err := calibrate(ctx, engine, suite, limit)
						result <- err
						return
					}
					_, _, err := run(ctx, engine, suite, Options{EvaluationDate: date, Modes: []retrieval.Mode{"exact"}}, limit)
					result <- err
				}()
				<-engine.started
				cancel()
				<-engine.cancelled
				close(engine.release)
				err := <-result
				if err == nil {
					t.Fatal("expected cancellation error")
				}
				got = append(got, err.Error())
				if engine.active.Load() != 0 {
					t.Fatal("worker still active")
				}
			}
			want := "collect case positive: collect failed: context canceled"
			for _, err := range got {
				if err != want {
					t.Fatalf("errors %q, want %q", got, want)
				}
			}
		})
	}
}

func TestOrderedPool_CancelledContextPreservesBufferedError(t *testing.T) {
	for i := 0; i < 100; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		expected := errors.New("case failure")
		pool := newOrderedPool(ctx, 1, 1, func(context.Context, int) error { return expected })
		buffered := <-pool.results[0]
		pool.results[0] <- buffered
		cancel()
		result := pool.take(0)
		pool.stop()
		if !errors.Is(result, expected) {
			t.Fatalf("result=%v, want %v", result, expected)
		}
	}
}
