package release

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type blockedAuth struct {
	entered, proceed chan struct{}
	once             sync.Once
}

func (s *blockedAuth) GetActiveRelease(ctx context.Context, _ string) (string, error) {
	s.once.Do(func() { close(s.entered) })
	select {
	case <-s.proceed:
		return "corpus_vn_20990103", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

type observedContext struct {
	context.Context
	once    sync.Once
	checked chan struct{}
}

func (c *observedContext) Err() error {
	err := c.Context.Err()
	c.once.Do(func() { close(c.checked) })
	return err
}

func TestResolver_AuthLookupDoesNotBlockDoneOrCancellation(t *testing.T) {
	for _, action := range []string{"done", "cancellation"} {
		t.Run(action, func(t *testing.T) {
			r, _, _ := resolverFixture(t)
			old := takeLease(t, r, "")
			auth := &blockedAuth{entered: make(chan struct{}), proceed: make(chan struct{})}
			r.Auth = auth
			lookupDone := make(chan struct{})
			go func() {
				defer close(lookupDone)
				lease, _ := r.Resolve(context.Background(), "vn", "")
				if lease != nil {
					lease.Done()
				}
			}()
			<-auth.entered
			defer func() { close(auth.proceed); <-lookupDone }()
			completed := make(chan error, 1)
			if action == "done" {
				go func() { old.Done(); completed <- nil }()
			} else {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				probe := &observedContext{Context: ctx, checked: make(chan struct{})}
				go func() {
					lease, err := r.Resolve(probe, "vn", "")
					if lease != nil {
						lease.Done()
					}
					completed <- err
				}()
				<-probe.checked
				cancel()
			}
			select {
			case err := <-completed:
				if action == "cancellation" && !errors.Is(err, context.Canceled) {
					t.Fatal("cancellation lost", err)
				}
			case <-time.After(30 * time.Millisecond):
				t.Fatal("auth I/O blocked lease bookkeeping or cancellation")
			}
		})
	}
}

type deadlineAuth struct{ bounded bool }

func (s *deadlineAuth) GetActiveRelease(ctx context.Context, _ string) (string, error) {
	deadline, ok := ctx.Deadline()
	s.bounded = ok && time.Until(deadline) > 0 && time.Until(deadline) <= 2*time.Second
	return "", errors.New("probe")
}
func TestResolver_AuthLookupHasTwoSecondDeadline(t *testing.T) {
	r, _, _ := resolverFixture(t)
	auth := &deadlineAuth{}
	r.Auth = auth
	_, _ = r.Resolve(context.Background(), "vn", "")
	if !auth.bounded {
		t.Fatal("auth lookup lacks the two-second deadline")
	}
}

func TestResolver_AuthLookupExpiresAtInjectedDeadline(t *testing.T) {
	r, _, _ := resolverFixture(t)
	auth := &blockedAuth{entered: make(chan struct{}), proceed: make(chan struct{})}
	r.Auth = auth
	r.QueryTimeout = 5 * time.Millisecond
	started := time.Now()
	_, err := r.Resolve(context.Background(), "vn", "")
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > 100*time.Millisecond {
		t.Fatal("lookup did not obey its deadline", err)
	}
}
