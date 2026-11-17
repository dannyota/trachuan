package loadrun

import (
	"context"
	"errors"
	"time"
)

func runActivations(ctx context.Context, duration time.Duration, offsets []time.Duration, hook func(context.Context, string) error) error {
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
	forwardErr := hook(ctx, "forward")
	if forwardErr == nil && ctx.Err() == nil {
		timer = time.NewTimer(time.Until(start.Add(offsets[1])))
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
	restoreCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	restoreErr := hook(restoreCtx, "restore")
	return errors.Join(forwardErr, ctx.Err(), restoreErr)
}
