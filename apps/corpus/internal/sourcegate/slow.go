package sourcegate

import (
	"context"
	"log/slog"
	"time"
)

// LogSlow records a slow source request without its URL or response contents.
func LogSlow(ctx context.Context, source, endpoint string, duration, threshold time.Duration, status int, err error) {
	if duration <= threshold {
		return
	}
	identity, _ := ctx.Value(itemKey{}).(string)
	outcome := "ok"
	if err != nil || status < 200 || status >= 300 {
		outcome = "error"
	}
	slog.Info("slow source request", "source", source, "endpoint", endpoint, "item", identity, "duration", duration, "outcome", outcome)
}
