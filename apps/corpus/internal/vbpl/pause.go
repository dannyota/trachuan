package vbpl

import (
	"context"
	"time"
)

type pauseContextKey struct{}
type pauseState struct {
	done   chan struct{}
	err    error
	reason string
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
	c.pauseMu.Unlock()
	if state == nil {
		return nil
	}
	select {
	case <-state.done:
		if state.err != nil {
			return c.persistPause(pauseParent(ctx), id, state.reason)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) persistPause(ctx context.Context, id int64, reason string) error {
	c.pauseMu.Lock()
	if c.pauses == nil {
		c.pauses = make(map[int64]*pauseState)
	}
	if state := c.pauses[id]; state != nil {
		select {
		case <-state.done:
			if state.err == nil {
				c.pauseMu.Unlock()
				return nil
			}
		default:
			c.pauseMu.Unlock()
			return c.awaitPause(ctx, id)
		}
	}
	state := &pauseState{done: make(chan struct{}), reason: reason}
	c.pauses[id] = state
	c.pauseMu.Unlock()
	pauseCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	state.err = c.gate.Pause(pauseCtx, id, "vbpl", reason, "")
	close(state.done)
	if state.err == nil {
		c.pauseMu.Lock()
		delete(c.pauses, id)
		c.pauseMu.Unlock()
	}
	return state.err
}
