package loadrun

import (
	"context"
	"testing"
	"time"
)

func TestActivation_RestoresAfterInterrupt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := runActivations(ctx, 2*time.Hour, []time.Duration{time.Millisecond, time.Hour}, func(context.Context) error {
		calls++
		if calls == 1 {
			cancel()
		}
		return nil
	})
	if err == nil || calls != 2 {
		t.Fatalf("calls %d, error %v", calls, err)
	}
}
