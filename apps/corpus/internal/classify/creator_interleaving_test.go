package classify

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type pausedCreatorRepository struct {
	Repository
	paused       chan Submission
	resume       chan struct{}
	releases     atomic.Int32
	afterRelease int32
	resumeOnce   sync.Once
}

func (p *pausedCreatorRepository) releaseCreator() { p.resumeOnce.Do(func() { close(p.resume) }) }

func (p *pausedCreatorRepository) Lock(ctx context.Context) (func(), error) {
	unlock, err := p.Repository.Lock(ctx)
	if err != nil {
		return nil, err
	}
	return func() {
		rows, _ := p.List(ctx)
		unlock()
		for _, s := range rows {
			if s.State != "preparing" || s.BatchID != "" || p.releases.Add(1) != p.afterRelease {
				continue
			}
			p.paused <- s
			select {
			case <-p.resume:
			case <-ctx.Done():
			}
			return
		}
	}, nil
}

func creatorInterleaving(t *testing.T, r *Runner, client *fakeClient, o Options, newRepository func() Repository, afterRelease int32) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var now atomic.Int64
	now.Store(r.Now().UnixNano())
	clock := func() time.Time { return time.Unix(0, now.Load()).UTC() }
	r.Now = clock
	pause := &pausedCreatorRepository{Repository: newRepository(), paused: make(chan Submission, 1), resume: make(chan struct{}), afterRelease: afterRelease}
	defer pause.releaseCreator()
	r.Repository = pause
	client.beforeCreate = nil
	client.ended = false
	done := make(chan error, 1)
	go func() { _, err := r.Screen(ctx, o, documents(), families()); done <- err }()
	var original Submission
	select {
	case original = <-pause.paused:
	case err := <-done:
		t.Fatalf("creator skipped the authorization boundary: %v", err)
	case <-ctx.Done():
		t.Fatal("creator did not pause before Create")
	}
	now.Store(original.CreatedAt.Add(25 * time.Hour).UnixNano())
	operator := &Runner{Repository: newRepository(), Client: client, IntentDir: r.IntentDir, Now: clock, Wait: r.Wait}
	if _, err := operator.Screen(ctx, o, documents(), families()); err == nil {
		t.Fatal("expected stale preparing attempt to become uncertain")
	}
	abandonErr := operator.Reconcile(ctx, original.ID, true)
	if abandonErr == nil {
		if _, err := operator.Screen(ctx, o, documents(), families()); err == nil {
			t.Fatal("expected interrupted replacement polling")
		}
	}
	pause.releaseCreator()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected original creator to stop")
		}
	case <-ctx.Done():
		t.Fatal("creator did not stop")
	}
	if abandonErr == nil {
		if client.creates != 1 {
			t.Fatalf("paused creator submitted after replacement: creates=%d", client.creates)
		}
		return
	}
	if afterRelease == 2 {
		if client.creates != 1 {
			t.Fatalf("expected only the leased original batch, creates=%d", client.creates)
		}
		if err := operator.Reconcile(ctx, original.ID, true); err == nil {
			t.Fatal("abandoned the submitted original batch")
		}
		if _, err := operator.Screen(ctx, o, documents(), families()); err == nil || client.creates != 1 {
			t.Fatal("submitted a replacement while the original batch remained active")
		}
		return
	}
	if client.creates != 0 {
		t.Fatalf("stale creator submitted: creates=%d", client.creates)
	}
	if err := operator.Reconcile(ctx, original.ID, true); err != nil {
		t.Fatal("finished creator still prevents abandonment", err)
	}
	if _, err := operator.Screen(ctx, o, documents(), families()); err == nil || client.creates != 1 {
		t.Fatal("replacement was not the only submitted batch")
	}
}

func TestScreen_PausedCreatorCannotSubmitAfterReplacement(t *testing.T) {
	for _, release := range []int32{1, 2} {
		t.Run(fmt.Sprint(release), func(t *testing.T) {
			r, ledger, client, o := screenFixture(t)
			creatorInterleaving(t, r, client, o, func() Repository { return ledger }, release)
		})
	}
}
