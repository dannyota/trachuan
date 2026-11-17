package classify

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

func countTokens(ctx context.Context, p anthropic.MessageNewParams, count TokenCounter) (int64, error) {
	return countTokensWithWait(ctx, p, count, wait)
}

func countTokensWithWait(ctx context.Context, p anthropic.MessageNewParams, count TokenCounter, pause func(context.Context, time.Duration) error) (int64, error) {
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		n, err := count(ctx, p)
		if err == nil {
			return n, nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return 0, ctxErr
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 0, err
		}
		var api *anthropic.Error
		var transport *url.Error
		if errors.As(err, &api) {
			if api.StatusCode < 500 || api.StatusCode >= 600 {
				return 0, err
			}
		} else if !errors.As(err, &transport) {
			return 0, err
		}
		if attempt == 3 {
			return 0, err
		}
		if err := pause(ctx, time.Duration(attempt)*time.Second); err != nil {
			return 0, err
		}
	}
}
