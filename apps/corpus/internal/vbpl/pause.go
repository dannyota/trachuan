package vbpl

import (
	"context"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
)

type pauseContextKey struct{}
type pauseState struct {
	done    chan struct{}
	reason  string
	running bool
}

func withPauseParent(ctx context.Context) context.Context {
	if ctx.Value(pauseContextKey{}) != nil {
		return ctx
	}
	return context.WithValue(ctx, pauseContextKey{}, ctx)
}

func pauseParent(ctx context.Context) context.Context {
	if parent, ok := ctx.Value(pauseContextKey{}).(context.Context); ok {
		return parent
	}
	return ctx
}

func (c *Client) awaitPause(ctx context.Context, id int64) error {
	c.pauseMu.Lock()
	state := c.pauses[id]
	if state != nil && !state.running && pauseParent(ctx).Err() == nil {
		state.running = true
		go c.pauseOwner(pauseParent(ctx), id, state)
	}
	c.pauseMu.Unlock()
	if state == nil {
		return nil
	}
	select {
	case <-state.done:
		return sourcegate.ErrSourcePaused
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) startPause(ctx context.Context, id int64, reason string) {
	c.pauseMu.Lock()
	defer c.pauseMu.Unlock()
	if c.pauses == nil {
		c.pauses = make(map[int64]*pauseState)
	}
	state := c.pauses[id]
	if state == nil {
		state = &pauseState{done: make(chan struct{}), reason: reason}
		c.pauses[id] = state
	}
	if !state.running {
		state.running = true
		go c.pauseOwner(ctx, id, state)
	}
}

func (c *Client) pauseOwner(ctx context.Context, id int64, state *pauseState) {
	delay := 2 * time.Second
	if len(c.backoffs) > 0 && c.backoffs[0] > 0 {
		delay = c.backoffs[0]
	}
	for ctx.Err() == nil {
		pauseCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := c.gate.Pause(pauseCtx, id, "vbpl", state.reason, "")
		cancel()
		if err == nil {
			c.pauseMu.Lock()
			close(state.done)
			delete(c.pauses, id)
			c.pauseMu.Unlock()
			return
		}
		if wait(ctx, delay) != nil {
			break
		}
	}
	c.pauseMu.Lock()
	state.running = false
	c.pauseMu.Unlock()
}
