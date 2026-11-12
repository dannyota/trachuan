package sourcegate

import (
	"context"
	"errors"
	"sync"
	"time"
)

// PauseWriter records a source pause in the workshop.
type PauseWriter interface {
	Pause(context.Context, int64, string, string, string) error
}

type pauseState struct {
	done, ownerDone chan struct{}
	reason          string
	running         bool
}

// Pauses blocks local admission while an activity owns a durable pause write.
// Its zero value is ready to use.
type Pauses struct {
	mu     sync.Mutex
	states map[sourceKey]*pauseState
}

// Start schedules persistence under the activity lifetime and returns a pause.
func (p *Pauses) Start(ctx context.Context, g PauseWriter, id int64, source, reason string, delay time.Duration) error {
	activity, ok := Activity(ctx)
	if !ok {
		return errors.New("source pause: missing activity context")
	}
	if err := activity.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.states == nil {
		p.states = map[sourceKey]*pauseState{}
	}
	key := sourceKey{id, source}
	state := p.states[key]
	if state == nil {
		state = &pauseState{done: make(chan struct{}), reason: reason}
		p.states[key] = state
	}
	p.start(activity, g, key, state, delay)
	return pausePending{}
}

// Await refuses admission while a pause is pending, respecting this waiter's cap.
func (p *Pauses) Await(ctx context.Context, g PauseWriter, id int64, source string, delay time.Duration) error {
	p.mu.Lock()
	key := sourceKey{id, source}
	state := p.states[key]
	if state == nil {
		p.mu.Unlock()
		return nil
	}
	activity, ok := Activity(ctx)
	if !ok {
		p.mu.Unlock()
		return errors.New("source pause: missing activity context")
	}
	if activity.Err() != nil {
		p.mu.Unlock()
		return activity.Err()
	}
	p.start(activity, g, key, state, delay)
	p.mu.Unlock()
	select {
	case <-state.done:
		return ErrSourcePaused
	case <-ctx.Done():
		if activity.Err() != nil {
			return activity.Err()
		}
		return pausePending{}
	}
}

func (p *Pauses) start(activity context.Context, g PauseWriter, key sourceKey, state *pauseState, delay time.Duration) {
	if state.running {
		return
	}
	state.running = true
	state.ownerDone = make(chan struct{})
	go p.owner(activity, g, key, state, delay, state.ownerDone)
}
func (p *Pauses) owner(activity context.Context, g PauseWriter, key sourceKey, state *pauseState, delay time.Duration, done chan struct{}) {
	defer close(done)
	if delay <= 0 {
		delay = 2 * time.Second
	}
	for activity.Err() == nil {
		writeCtx, cancel := context.WithTimeout(activity, 10*time.Second)
		err := g.Pause(writeCtx, key.runID, key.source, state.reason, "")
		cancel()
		if err == nil {
			p.mu.Lock()
			close(state.done)
			delete(p.states, key)
			p.mu.Unlock()
			return
		}
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-activity.Done():
		}
		timer.Stop()
	}
	p.mu.Lock()
	state.running = false
	p.mu.Unlock()
}

type pausePending struct{}

func (pausePending) Error() string      { return "source pause persistence pending" }
func (pausePending) Unwrap() error      { return ErrSourcePaused }
func (pausePending) PausePending() bool { return true }
