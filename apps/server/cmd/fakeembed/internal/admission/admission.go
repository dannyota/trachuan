// Package admission bounds concurrent work and its queue.
package admission

import (
	"context"
	"errors"
	"time"
)

// ErrBusy indicates that capacity or queue wait was exhausted.
var ErrBusy = errors.New("admission busy")

// ErrDeadline indicates that the work deadline expired after admission.
var ErrDeadline = errors.New("admission deadline exceeded")

// Limits bounds active work, queued requests and their duration.
type Limits struct {
	Active, Queued int
	Wait, Deadline time.Duration
}

// QueryLimits returns the query service admission limits.
func QueryLimits() Limits {
	return Limits{Active: 2, Queued: 8, Wait: 250 * time.Millisecond, Deadline: 2 * time.Second}
}

// Admission controls access to bounded work.
type Admission struct {
	limits   Limits
	capacity chan struct{}
	active   chan struct{}
}

// New creates an admission controller with the given limits.
func New(l Limits) *Admission {
	return &Admission{limits: l, capacity: make(chan struct{}, l.Active+l.Queued), active: make(chan struct{}, l.Active)}
}

// Do runs work after admission, returning when it finishes or the caller expires.
func (a *Admission) Do(ctx context.Context, work func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case a.capacity <- struct{}{}:
	default:
		return ErrBusy
	}
	workCtx, cancel := context.WithTimeout(ctx, a.limits.Deadline)
	defer cancel()
	queueDeadline := time.Now().Add(a.limits.Wait)
	timer := time.NewTimer(time.Until(queueDeadline))
	defer timer.Stop()
	select {
	case a.active <- struct{}{}:
		if workCtx.Err() != nil || !time.Now().Before(queueDeadline) {
			<-a.active
			<-a.capacity
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return ErrBusy
		}
	case <-timer.C:
		<-a.capacity
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ErrBusy
	case <-workCtx.Done():
		<-a.capacity
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ErrBusy
	}
	result := make(chan error, 1)
	go func() {
		err := work(workCtx)
		<-a.active
		<-a.capacity
		result <- err
	}()
	select {
	case err := <-result:
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if workCtx.Err() != nil {
			return ErrDeadline
		}
		return err
	case <-workCtx.Done():
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ErrDeadline
	}
}
