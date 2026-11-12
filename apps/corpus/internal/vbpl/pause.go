package vbpl

import (
	"context"
	"errors"
	"strings"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
)

func (c *Client) pauseDelay() time.Duration {
	if len(c.backoffs) > 0 && c.backoffs[0] > 0 {
		return c.backoffs[0]
	}
	return 2 * time.Second
}
func (c *Client) awaitPause(ctx context.Context, id int64) error {
	return c.pauses.Await(ctx, c.gate, id, "vbpl", c.pauseDelay())
}
func (c *Client) deny(ctx context.Context, reason string) error {
	if c.gate == nil {
		return sourcegate.ErrSourcePaused
	}
	id, ok := sourcegate.RunID(ctx)
	if !ok {
		return errors.New("source pause: missing run id")
	}
	return c.pauses.Start(ctx, c.gate, id, "vbpl", reason, c.pauseDelay())
}

func challenge(head []byte) bool {
	text := strings.ToLower(string(head))
	if !strings.Contains(text, "<html") && !strings.Contains(text, "<script") && !strings.Contains(text, "<!doctype html") {
		return false
	}
	return strings.Contains(text, "cf-chl-") || strings.Contains(text, "captcha") || strings.Contains(text, "verify you are human") || strings.Contains(text, "just a moment")
}
