package loadrun

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestActivation_RestoresAfterFailedForward(t *testing.T) {
	current := "original"
	err := runActivations(context.Background(), time.Second, []time.Duration{time.Millisecond, 2 * time.Millisecond}, func(ctx context.Context, target string) error {
		if target == "forward" {
			current = "alternate"
			return errors.New("lost outcome")
		}
		if target != "restore" || ctx.Err() != nil {
			t.Fatal("invalid restore")
		}
		current = "original"
		return nil
	})
	if err == nil || current != "original" {
		t.Fatalf("release %s, error %v", current, err)
	}
}
func TestActivation_RestoresAfterInterrupt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	current := "original"
	calls := 0
	forwardDone := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		finished <- runActivations(ctx, 2*time.Hour, []time.Duration{time.Millisecond, time.Hour}, func(hookCtx context.Context, target string) error {
			calls++
			if target == "forward" {
				current = "alternate"
				close(forwardDone)
				return nil
			}
			if target != "restore" || hookCtx.Err() != nil {
				return errors.New("restore context or target invalid")
			}
			current = "original"
			return nil
		})
	}()
	select {
	case <-forwardDone:
	case <-time.After(80 * time.Millisecond):
		t.Fatal("forward hook did not run")
	}
	cancel()
	err := <-finished
	if err == nil || calls != 2 || current != "original" {
		t.Fatalf("calls %d release %s error %v", calls, current, err)
	}
}
