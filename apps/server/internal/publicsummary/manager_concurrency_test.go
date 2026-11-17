package publicsummary

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func receivePromptly[T any](t *testing.T, result <-chan T, message string) (T, bool) {
	t.Helper()
	select {
	case value := <-result:
		return value, true
	case <-time.After(100 * time.Millisecond):
		t.Error(message)
		var zero T
		return zero, false
	}
}

func TestManager_SlowMetadataDoesNotBlockCancellationOrRetirement(t *testing.T) {
	entered, finish := make(chan struct{}), make(chan struct{})
	var calls atomic.Int64
	var expire func()
	m := NewManager(func(context.Context) (Release, error) {
		call := calls.Add(1)
		if call == 3 {
			close(entered)
			<-finish
		}
		name := "20990101"
		if call > 1 {
			name = "20990102"
		}
		return Release{Database: "corpus_vn_" + name, ID: name, Projection: 1}, nil
	}, func(context.Context, string) (Pool, error) { return &memoryPool{closed: make(chan struct{})}, nil }, time.Now)
	m.after = func(_ time.Duration, f func()) { expire = f }
	old, err := m.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	next, err := m.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		lease, e := m.Resolve(ctx)
		if lease != nil {
			lease.Done()
		}
		result <- e
	}()
	<-entered
	cancel()
	if err, ok := receivePromptly(t, result, "cancelled metadata caller blocked"); ok && !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled result: %v", err)
	}
	retired := make(chan struct{})
	go func() { expire(); close(retired) }()
	receivePromptly(t, retired, "retirement waited for metadata")
	done := make(chan struct{})
	go func() { old.Done(); next.Done(); close(done) }()
	receivePromptly(t, done, "lease completion waited for metadata")
	close(finish)
	m.Close()
}

func TestManager_LateOpeningCannotReplaceNewerRelease(t *testing.T) {
	entered, finish := make(chan struct{}), make(chan struct{})
	oldPool := &memoryPool{closed: make(chan struct{})}
	var calls atomic.Int64
	m := NewManager(func(context.Context) (Release, error) {
		name := "20990101"
		if calls.Add(1) > 1 {
			name = "20990102"
		}
		return Release{Database: "corpus_vn_" + name, ID: name, Projection: 1}, nil
	}, func(_ context.Context, name string) (Pool, error) {
		if name == "corpus_vn_20990101" {
			close(entered)
			<-finish
			return oldPool, nil
		}
		return &memoryPool{closed: make(chan struct{})}, nil
	}, time.Now)
	first := make(chan *Lease, 1)
	go func() { lease, _ := m.Resolve(context.Background()); first <- lease }()
	<-entered
	second := make(chan *Lease, 1)
	go func() { lease, _ := m.Resolve(context.Background()); second <- lease }()
	current, ok := receivePromptly(t, second, "new release waited for old opening")
	close(finish)
	late, _ := receivePromptly(t, first, "old opener did not finish")
	if ok && (current == nil || current.Release.ID != "20990102") {
		t.Error("new release not published")
	}
	if late != nil {
		late.Done()
	}
	if current != nil {
		current.Done()
	}
	m.mu.Lock()
	if m.active == nil || m.active.release.ID != "20990102" {
		t.Error("late old result replaced active release")
	}
	m.mu.Unlock()
	m.Close()
}

func TestSummary_CancelledMetadataWaitersReleaseAdmission(t *testing.T) {
	h, _, _, _ := handlerFixture(t)
	entered, finish := make(chan struct{}), make(chan struct{})
	h.manager.metadata = func(context.Context) (Release, error) {
		select {
		case <-entered:
		default:
			close(entered)
		}
		<-finish
		return Release{}, ErrUnavailable
	}
	ctx, cancel := context.WithCancel(context.Background())
	returned := make(chan struct{}, 8)
	for range 8 {
		go func() {
			defer func() { returned <- struct{}{} }()
			request := httptest.NewRequest("GET", "/api/v1/public/documents", nil).WithContext(ctx)
			h.ServeHTTP(httptest.NewRecorder(), request)
		}()
	}
	<-entered
	deadline := time.Now().Add(100 * time.Millisecond)
	for len(h.gate) != 8 && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	if len(h.gate) != 8 {
		t.Error("requests did not enter admission")
	}
	cancel()
	drained := make(chan struct{})
	go func() {
		for range 8 {
			<-returned
		}
		close(drained)
	}()
	receivePromptly(t, drained, "disconnected callers held admission while metadata stalled")
	if len(h.gate) != 0 {
		t.Error("admission slots still held")
	}
	close(finish)
}

type heldClosePool struct {
	memoryPool
	started chan struct{}
	finish  <-chan struct{}
}

func (p *heldClosePool) Close() { close(p.started); <-p.finish; p.memoryPool.Close() }

func TestManager_SlowClosureDoesNotBlockStateAndCapsResources(t *testing.T) {
	finish := make(chan struct{})
	defer close(finish)
	pools := []*heldClosePool{
		{memoryPool: memoryPool{closed: make(chan struct{})}, started: make(chan struct{}), finish: finish},
		{memoryPool: memoryPool{closed: make(chan struct{})}, started: make(chan struct{}), finish: finish},
	}
	var observations, opens atomic.Int64
	m := NewManager(func(context.Context) (Release, error) {
		n := observations.Add(1)
		name := fmt.Sprintf("2099010%d", n)
		return Release{Database: "corpus_vn_" + name, ID: name, Projection: 1}, nil
	}, func(context.Context, string) (Pool, error) {
		n := opens.Add(1)
		if n > 2 {
			return &memoryPool{closed: make(chan struct{})}, nil
		}
		return pools[n-1], nil
	}, time.Now)
	first, err := m.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	first.Done()
	second, err := m.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second.Done()
	receivePromptly(t, pools[0].started, "old pool did not begin closing")
	third := make(chan error, 1)
	go func() {
		lease, e := m.Resolve(context.Background())
		if lease != nil {
			lease.Done()
		}
		third <- e
	}()
	receivePromptly(t, pools[1].started, "second pool did not retire")
	closed := make(chan struct{})
	go func() { m.Close(); close(closed) }()
	receivePromptly(t, closed, "manager close waited for a stalled pool close")
	if err, ok := receivePromptly(t, third, "request waited on stalled closing pools"); ok && err == nil {
		t.Error("request succeeded without a connection resource")
	}
	if opens.Load() != 2 {
		t.Errorf("opened %d pools while two resources were still closing", opens.Load())
	}
}
