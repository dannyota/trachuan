package password

import (
	"context"
	"time"
)

type failure string

func (e failure) Error() string { return string(e) }

const ErrBusy failure = "password hashing busy"

type Admission struct {
	slots, queue chan struct{}
	wait         time.Duration
}

func NewAdmission() *Admission {
	return &Admission{slots: make(chan struct{}, 2), queue: make(chan struct{}, 16), wait: 2 * time.Second}
}
func (a *Admission) Acquire(ctx context.Context) error {
	if ctx.Err() != nil {
		return ErrBusy
	}
	select {
	case a.slots <- struct{}{}:
		return nil
	default:
	}
	select {
	case a.queue <- struct{}{}:
	default:
		return ErrBusy
	}
	defer func() { <-a.queue }()
	timer := time.NewTimer(a.wait)
	defer timer.Stop()
	select {
	case a.slots <- struct{}{}:
		if ctx.Err() != nil {
			a.Release()
			return ErrBusy
		}
		return nil
	case <-ctx.Done():
		return ErrBusy
	case <-timer.C:
		return ErrBusy
	}
}
func (a *Admission) Release() { <-a.slots }
