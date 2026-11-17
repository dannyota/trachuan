package release

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"danny.vn/trachuan/internal/frameworks/serve"
	"github.com/stretchr/testify/require"
)

func TestResolver_FrameworksRequestsNeverOpen(t *testing.T) {
	var opens atomic.Int32
	r := &Resolver{Auth: &fakeStore{active: "frameworks_20990103"}, Clock: time.Now, Frameworks: &serve.Options{}, OpenFrameworks: func(ctx context.Context, _ string) (*serve.Engine, func(), error) {
		opens.Add(1)
		<-ctx.Done()
		return nil, nil, ctx.Err()
	}}
	t.Cleanup(r.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	lease, err := r.Resolve(ctx, "vn", "")
	require.Nil(t, lease)
	require.ErrorIs(t, err, serve.ErrUnavailable)
	require.Zero(t, opens.Load())
}

func TestResolver_FrameworksWatcherOpensColdStartAndActivation(t *testing.T) {
	s := &fakeStore{active: "frameworks_20990103"}
	entered, proceed := make(chan string, 2), make(chan struct{}, 2)
	var opens atomic.Int32
	r := &Resolver{Auth: s, Clock: time.Now, Frameworks: &serve.Options{}, OpenFrameworks: func(ctx context.Context, name string) (*serve.Engine, func(), error) {
		opens.Add(1)
		entered <- name
		select {
		case <-proceed:
			return &serve.Engine{}, nil, nil
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
	}}
	ctx, cancel := context.WithCancel(t.Context())
	ticks, done := make(chan time.Time), make(chan struct{})
	go func() { defer close(done); r.watchFrameworks(ctx, ticks) }()
	t.Cleanup(func() { cancel(); <-done; r.Close() })
	for _, name := range []string{"frameworks_20990103", "frameworks_20990104"} {
		if name == "frameworks_20990104" {
			s.mu.Lock()
			s.active = name
			s.mu.Unlock()
			_, err := r.Resolve(t.Context(), "vn", "")
			require.ErrorIs(t, err, serve.ErrUnavailable)
			require.Equal(t, int32(1), opens.Load())
			ticks <- time.Time{}
		}
		require.Equal(t, name, <-entered)
		probe, stop := context.WithTimeout(t.Context(), 20*time.Millisecond)
		lease, err := r.Resolve(probe, "vn", "")
		stop()
		require.Nil(t, lease)
		require.ErrorIs(t, err, serve.ErrUnavailable)
		proceed <- struct{}{}
		waitFrameworksReady(t, r, name)
		lease, err = r.Resolve(t.Context(), "vn", "")
		require.NoError(t, err)
		require.NotNil(t, lease.Frameworks)
		lease.Done()
	}
	require.Equal(t, int32(2), opens.Load())
}

func waitFrameworksReady(t *testing.T, r *Resolver, name string) {
	t.Helper()
	deadline := time.NewTimer(50 * time.Millisecond)
	defer deadline.Stop()
	for {
		r.mu.Lock()
		entry := r.entries[name]
		ready := entry != nil && !entry.opening && (entry.frameworks != nil || entry.openErr != nil)
		r.mu.Unlock()
		if ready {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("background open did not finish")
		default:
			runtime.Gosched()
		}
	}
}

func TestResolver_FrameworksFailedOpenStaysUntilPointerChanges(t *testing.T) {
	s := &fakeStore{active: "frameworks_20990103"}
	var opens, closes atomic.Int32
	r := &Resolver{Auth: s, Clock: time.Now, Frameworks: &serve.Options{}, OpenFrameworks: func(context.Context, string) (*serve.Engine, func(), error) {
		opens.Add(1)
		return &serve.Engine{}, func() { closes.Add(1) }, errors.New("invented rejected release")
	}}
	t.Cleanup(r.Close)
	for _, name := range []string{"frameworks_20990103", "frameworks_20990104", "frameworks_20990103"} {
		s.mu.Lock()
		s.active = name
		s.mu.Unlock()
		result := r.refreshFrameworks(t.Context())
		require.NotNil(t, result)
		require.Error(t, (<-result).Err)
		before := opens.Load()
		for range 3 {
			lease, err := r.Resolve(t.Context(), "vn", "")
			require.Nil(t, lease)
			require.ErrorIs(t, err, serve.ErrUnavailable)
			require.Nil(t, r.refreshFrameworks(t.Context()))
		}
		require.Equal(t, before, opens.Load())
		waitResolverCleanup(r)
		require.Equal(t, before, closes.Load())
	}
	require.Equal(t, int32(3), opens.Load())
	r.Close()
	restarted := &Resolver{Auth: s, Clock: time.Now, Frameworks: &serve.Options{}, OpenFrameworks: r.OpenFrameworks}
	t.Cleanup(restarted.Close)
	require.Error(t, (<-restarted.refreshFrameworks(t.Context())).Err)
	require.Equal(t, int32(4), opens.Load())
}

func TestResolver_FrameworksWatcherCancelsPendingOpen(t *testing.T) {
	entered, done := make(chan struct{}), make(chan struct{})
	var closed atomic.Int32
	r := &Resolver{Auth: &fakeStore{active: "frameworks_20990103"}, Clock: time.Now, Frameworks: &serve.Options{}, OpenFrameworks: func(ctx context.Context, _ string) (*serve.Engine, func(), error) {
		close(entered)
		<-ctx.Done()
		return nil, func() { closed.Add(1) }, ctx.Err()
	}}
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(r.Close)
	t.Cleanup(cancel)
	go func() { defer close(done); r.watchFrameworks(ctx, make(chan time.Time)) }()
	<-entered
	cancel()
	select {
	case <-done:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("watcher did not cancel its pending open")
	}
	r.Close()
	require.Equal(t, int32(1), closed.Load())
}
