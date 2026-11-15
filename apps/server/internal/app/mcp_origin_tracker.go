package app

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type mcpOriginTracker struct {
	mu       sync.Mutex
	clock    func() time.Time
	logger   *slog.Logger
	hour     time.Time
	origins  map[string]struct{}
	total    uint64
	overflow uint64
}

func (t *mcpOriginTracker) observe(ctx context.Context, outside bool, values []string, path, mode string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	hour := t.clock().UTC().Truncate(time.Hour)
	if !hour.Equal(t.hour) {
		if t.total > 0 {
			t.logger.InfoContext(ctx, "MCP origin summary", "total", t.total, "distinct", len(t.origins), "overflow", t.overflow)
		}
		t.hour = hour
		t.origins = make(map[string]struct{})
		t.total, t.overflow = 0, 0
	}
	if !outside {
		return
	}
	origin := mcpOriginClass(values)
	t.total++
	if _, seen := t.origins[origin]; seen {
		return
	}
	if len(t.origins) == 100 {
		t.overflow++
		return
	}
	t.origins[origin] = struct{}{}
	t.logger.InfoContext(ctx, "MCP origin outside allowlist", "origin", origin, "path", path, "mode", mode)
}
