package worker

import (
	"context"
	"errors"
	"time"

	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
)

type retryEmbedder struct {
	embedder retrieval.Embedder
}

func (r retryEmbedder) Recipe(ctx context.Context) (embedding.Recipe, error) {
	return r.embedder.Recipe(ctx)
}

func (r retryEmbedder) Embed(ctx context.Context, text string) (embedding.Vector, error) {
	waits := [...]time.Duration{250 * time.Millisecond, time.Second, 3 * time.Second}
	for attempt := 0; ; attempt++ {
		vector, err := r.embedder.Embed(ctx, text)
		var service *embedding.Error
		if err == nil || !errors.As(err, &service) || (service.Status != 429 && service.Code != "busy") || attempt == len(waits) {
			return vector, err
		}
		wait := waits[attempt]
		if service.RetryAfter != nil {
			seconds := *service.RetryAfter
			if seconds >= 5 {
				wait = 5 * time.Second
			} else if seconds > 0 && time.Duration(seconds)*time.Second > wait {
				wait = time.Duration(seconds) * time.Second
			}
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return embedding.Vector{}, ctx.Err()
		case <-timer.C:
		}
	}
}
