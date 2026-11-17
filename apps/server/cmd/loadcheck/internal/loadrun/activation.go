package loadrun

import (
	"context"
	"errors"
	"time"
)

func runActivations(ctx context.Context, duration time.Duration, offsets []time.Duration, hook func(context.Context) error) error {
	if len(offsets) != 2 || duration < offsets[1] {
		return nil
	}
	start := time.Now()
	timer := time.NewTimer(time.Until(start.Add(offsets[0])))
	select {
	case <-ctx.Done():
		timer.Stop()
		return ctx.Err()
	case <-timer.C:
	}
	if err := hook(ctx); err != nil {
		return err
	}
	timer = time.NewTimer(time.Until(start.Add(offsets[1])))
	select {
	case <-ctx.Done():
		timer.Stop()
		restoreCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return errors.Join(ctx.Err(), hook(restoreCtx))
	case <-timer.C:
		return hook(ctx)
	}
}
