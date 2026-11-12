package sourcegate

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

type waitingPauseGate struct {
	*Memory
	calls  int
	allow  chan struct{}
	bounds []time.Duration
}

func (g *waitingPauseGate) Pause(ctx context.Context, id int64, source, reason, evidence string) error {
	g.calls++
	deadline, _ := ctx.Deadline()
	g.bounds = append(g.bounds, time.Until(deadline))
	select {
	case <-g.allow:
		return g.Memory.Pause(ctx, id, source, reason, evidence)
	case <-ctx.Done():
		return ctx.Err()
	}
}
func TestPause_PendingWriteBlocksAdmissionAcrossWriteDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		activity, cancel := context.WithCancel(context.Background())
		defer cancel()
		activity = WithActivity(activity)
		request, stop := context.WithTimeout(activity, time.Second)
		defer stop()
		g := &waitingPauseGate{Memory: NewMemory(), allow: make(chan struct{})}
		var pauses Pauses
		if err := pauses.Start(request, g, 1, "chinhphu", "http 403", 2*time.Second); !errors.Is(err, ErrSourcePaused) {
			t.Fatal(err)
		}
		synctest.Wait()
		if err := pauses.Await(request, g, 1, "chinhphu", 2*time.Second); !errors.Is(err, ErrSourcePaused) || activity.Err() != nil {
			t.Fatalf("wait=%v activity=%v", err, activity.Err())
		}
		time.Sleep(11 * time.Second)
		synctest.Wait()
		if g.calls != 2 {
			t.Fatalf("write attempts=%d", g.calls)
		}
		for _, bound := range g.bounds {
			if bound != 10*time.Second {
				t.Fatalf("write deadline=%s", bound)
			}
		}
		waiter, stopWait := context.WithTimeout(activity, time.Second)
		defer stopWait()
		if err := pauses.Await(waiter, g, 1, "chinhphu", 2*time.Second); !errors.Is(err, ErrSourcePaused) {
			t.Fatalf("admission while pending=%v", err)
		}
		close(g.allow)
		synctest.Wait()
		if err := g.Admit(activity, 1, "chinhphu"); !errors.Is(err, ErrSourcePaused) {
			t.Fatalf("durable admission=%v", err)
		}
	})
}

func TestPause_OwnerGoroutineStopsWithActivity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		activity, cancel := context.WithCancel(context.Background())
		activity = WithActivity(activity)
		g := &waitingPauseGate{Memory: NewMemory(), allow: make(chan struct{})}
		var pauses Pauses
		_ = pauses.Start(activity, g, 1, "vbpl", "http 403", time.Second)
		synctest.Wait()
		pauses.mu.Lock()
		done := pauses.states[sourceKey{1, "vbpl"}].ownerDone
		pauses.mu.Unlock()
		cancel()
		<-done
		if err := pauses.Await(activity, g, 1, "vbpl", time.Second); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation=%v", err)
		}
	})
}
