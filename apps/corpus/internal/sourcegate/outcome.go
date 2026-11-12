package sourcegate

import (
	"context"
	"errors"
)

// OutcomeContextError propagates activity cancellation. An inner deadline cannot
// replace a status fixed by headers or a challenge pause found in the body.
// Success is the status declared by this request type.
func OutcomeContextError(ctx context.Context, status, success int, outcome error) error {
	lifetime := ctx
	if activity, ok := Activity(ctx); ok {
		lifetime = activity
	}
	if err := lifetime.Err(); err != nil {
		return err
	}
	if errors.Is(outcome, ErrSourcePaused) || status != 0 && status != success {
		return nil
	}
	return ctx.Err()
}
