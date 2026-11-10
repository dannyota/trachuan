package admission

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestQueryLimits_Values(t *testing.T) {
	if got := QueryLimits(); got != (Limits{2, 8, 250 * time.Millisecond, 2 * time.Second}) {
		t.Fatalf("limits: %+v", got)
	}
}

func startWork(a *Admission) (chan struct{}, <-chan error) {
	started, release, result := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		result <- a.Do(context.Background(), func(context.Context) error { close(started); <-release; return nil })
	}()
	<-started
	return release, result
}

func TestDo_BusyWhenActiveAndQueueFull(t *testing.T) {
	a := New(Limits{1, 0, time.Second, time.Second})
	release, result := startWork(a)
	if err := a.Do(context.Background(), func(context.Context) error { return nil }); !errors.Is(err, ErrBusy) {
		t.Fatalf("got %v", err)
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatalf("active work: %v", err)
	}
}

func TestDo_BusyAfterQueueWait(t *testing.T) {
	a := New(Limits{1, 1, 10 * time.Millisecond, time.Second})
	release, result := startWork(a)
	if err := a.Do(context.Background(), func(context.Context) error { t.Error("work started"); return nil }); !errors.Is(err, ErrBusy) {
		t.Fatalf("got %v", err)
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatalf("active work: %v", err)
	}
}

func TestDo_DeadlineReturnsButSlotHeldUntilWorkEnds(t *testing.T) {
	a := New(Limits{1, 0, time.Second, 15 * time.Millisecond})
	release, result := startWork(a)
	if err := <-result; !errors.Is(err, ErrDeadline) {
		t.Fatalf("got %v", err)
	}
	if err := a.Do(context.Background(), func(context.Context) error { return nil }); !errors.Is(err, ErrBusy) {
		t.Fatalf("got %v", err)
	}
	close(release)
}

func TestDo_WorkContextCancelledAtDeadline(t *testing.T) {
	a := New(Limits{1, 0, time.Second, 10 * time.Millisecond})
	cancelled := make(chan struct{})
	err := a.Do(context.Background(), func(ctx context.Context) error { <-ctx.Done(); close(cancelled); return ctx.Err() })
	if !errors.Is(err, ErrDeadline) {
		t.Fatalf("got %v", err)
	}
	<-cancelled
}

func TestDo_CancelledWhileQueuedFreesQueueSlot(t *testing.T) {
	a := New(Limits{1, 1, time.Second, time.Second})
	release, result := startWork(a)
	ctx, cancel := context.WithCancel(context.Background())
	queued := make(chan error, 1)
	go func() { queued <- a.Do(ctx, func(context.Context) error { t.Error("work started"); return nil }) }()
	// Wait until the queued call occupies capacity.
	deadline := time.After(50 * time.Millisecond)
	for len(a.capacity) != 2 {
		select {
		case <-deadline:
			t.Fatal("queue did not fill")
		default:
		}
	}
	cancel()
	if err := <-queued; !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if len(a.capacity) != 1 {
		t.Fatal("queue slot retained")
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatalf("active work: %v", err)
	}
	if err := a.Do(context.Background(), func(context.Context) error { return nil }); err != nil {
		t.Fatalf("got %v", err)
	}
}

func TestDo_WorkErrorReturned(t *testing.T) {
	want := errors.New("work failed")
	a := New(Limits{1, 0, time.Second, time.Second})
	if err := a.Do(context.Background(), func(context.Context) error { return want }); !errors.Is(err, want) {
		t.Fatalf("got %v", err)
	}
}

func TestDo_DeadlineWhileQueuedIsBusy(t *testing.T) {
	a := New(Limits{1, 1, time.Second, 10 * time.Millisecond})
	release, result := startWork(a)
	if err := a.Do(context.Background(), func(context.Context) error { t.Error("work started"); return nil }); !errors.Is(err, ErrBusy) {
		t.Fatalf("got %v", err)
	}
	close(release)
	if err := <-result; err != nil && !errors.Is(err, ErrDeadline) {
		t.Fatalf("active work: %v", err)
	}
}

func TestDo_CallerCancellationDuringWork(t *testing.T) {
	a := New(Limits{1, 0, time.Second, time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- a.Do(ctx, func(workCtx context.Context) error {
			close(started)
			<-workCtx.Done()
			<-release
			close(finished)
			return nil
		})
	}()
	<-started
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if err := a.Do(context.Background(), func(context.Context) error { return nil }); !errors.Is(err, ErrBusy) {
		t.Fatalf("got %v", err)
	}
	close(release)
	<-finished
}
