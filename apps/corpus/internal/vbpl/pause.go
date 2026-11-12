package vbpl

import (
	"context"
	"errors"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
)

type pauseState struct {
	done      chan struct{}
	reason    string
	running   bool
	ownerDone chan struct{}
}

func (c *Client) awaitPause(ctx context.Context, id int64) error {
	c.pauseMu.Lock()
	state := c.pauses[id]
	activity, ok := sourcegate.Activity(ctx)
	if state != nil && !ok {
		c.pauseMu.Unlock()
		return errors.New("source pause: missing activity context")
	}
	if state != nil && !state.running && activity.Err() == nil {
		state.running = true
		state.ownerDone = make(chan struct{})
		go c.pauseOwner(activity, id, state, state.ownerDone)
	}
	c.pauseMu.Unlock()
	if state == nil {
		return nil
	}
	select {
	case <-state.done:
		return sourcegate.ErrSourcePaused
	case <-ctx.Done():
		return pausePending{}
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
		state.ownerDone = make(chan struct{})
		go c.pauseOwner(ctx, id, state, state.ownerDone)
	}
}

func (c *Client) pauseOwner(ctx context.Context, id int64, state *pauseState, done chan struct{}) {
	defer close(done)
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

// pausePending keeps an unpersisted source pause distinct from a request timeout.
type pausePending struct{}

func (pausePending) Error() string      { return "source pause persistence pending" }
func (pausePending) Unwrap() error      { return sourcegate.ErrSourcePaused }
func (pausePending) PausePending() bool { return true }
