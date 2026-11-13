package release

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestResolver_RetiredCleanupDoesNotBlockRequest(t *testing.T) {
	for _, lastLease := range []bool{false, true} {
		t.Run(map[bool]string{false: "activation", true: "last lease"}[lastLease], func(t *testing.T) {
			r, store, _ := resolverFixture(t)
			entered, release := make(chan struct{}), make(chan struct{})
			withPoolCloser(r, func(string) { close(entered); <-release })
			defer r.Close()
			defer close(release)
			lease := takeLease(t, r, "")
			if !lastLease {
				lease.Done()
			}
			store.mu.Lock()
			store.active = ""
			store.mu.Unlock()
			result := make(chan error, 1)
			activate := func() error { _, err := r.Resolve(context.Background(), "vn", ""); return err }
			if lastLease {
				if err := activate(); !errors.Is(err, ErrExpired) {
					t.Fatal(err)
				}
				go func() { lease.Done(); result <- nil }()
			} else {
				go func() { result <- activate() }()
			}
			<-entered
			select {
			case err := <-result:
				if !lastLease && !errors.Is(err, ErrExpired) {
					t.Fatal(err)
				}
			case <-time.After(50 * time.Millisecond):
				t.Fatal("request waited for retired cleanup")
			}
		})
	}
}

func TestResolver_CloseWaitsForRetiredCleanup(t *testing.T) {
	r, store, _ := resolverFixture(t)
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	withPoolCloser(r, func(string) { close(entered); <-release })
	takeLease(t, r, "").Done()
	store.mu.Lock()
	store.active = ""
	store.mu.Unlock()
	activation := make(chan struct{})
	go func() { _, _ = r.Resolve(context.Background(), "vn", ""); close(activation) }()
	<-entered
	go func() { r.Close(); close(finished) }()
	select {
	case <-finished:
		t.Error("Close returned before cleanup")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	<-finished
	<-activation
}

func waitResolverCleanup(r *Resolver) {
	r.mu.Lock()
	var done []chan struct{}
	for _, cleanup := range r.cleanups {
		done = append(done, cleanup)
	}
	r.mu.Unlock()
	for _, cleanup := range done {
		<-cleanup
	}
}

func TestResolver_CloseWaitsForCleanupQueuedDuringShutdown(t *testing.T) {
	r, store, _ := resolverFixture(t)
	enteredA, enteredB, releaseA, releaseB := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	var stopA, stopB sync.Once
	defer r.Close()
	defer stopA.Do(func() { close(releaseA) })
	defer stopB.Do(func() { close(releaseB) })
	withPoolCloser(r, func(name string) {
		if name == "corpus_vn_20990103" {
			close(enteredA)
			<-releaseA
		} else {
			close(enteredB)
			<-releaseB
		}
	})
	takeLease(t, r, "").Done()
	store.mu.Lock()
	store.active = "corpus_vn_20990104"
	store.mu.Unlock()
	lease := takeLease(t, r, "")
	<-enteredA
	shutdown, finished := make(chan struct{}), make(chan struct{})
	var notified sync.Once
	r.mu.Lock()
	cancel := r.cancelWork
	r.cancelWork = func() { cancel(); notified.Do(func() { close(shutdown) }) }
	r.mu.Unlock()
	go func() { r.Close(); close(finished) }()
	<-shutdown
	released := make(chan struct{})
	go func() { lease.Done(); close(released) }()
	<-enteredB
	stopA.Do(func() { close(releaseA) })
	select {
	case <-finished:
		t.Error("Close skipped cleanup queued during shutdown")
	case <-time.After(20 * time.Millisecond):
	}
	stopB.Do(func() { close(releaseB) })
	<-finished
	<-released
}

func TestResolver_ClosedFinalLeaseWaitsForCleanup(t *testing.T) {
	r, _, _ := resolverFixture(t)
	entered, release, finished, closed := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	withPoolCloser(r, func(string) { close(entered); <-release; close(closed) })
	defer r.Close()
	var stop sync.Once
	defer stop.Do(func() { close(release) })
	lease := takeLease(t, r, "")
	r.Close()
	go func() { lease.Done(); close(finished) }()
	<-entered
	select {
	case <-finished:
		t.Error("final lease returned before shutdown cleanup")
	case <-time.After(20 * time.Millisecond):
	}
	stop.Do(func() { close(release) })
	<-finished
	select {
	case <-closed:
	default:
		t.Fatal("final lease did not close pool")
	}
}

func TestResolver_ClosedFinalLeaseWaitsForPendingOpen(t *testing.T) {
	r, _, _ := resolverFixture(t)
	closed := make(chan struct{})
	withPoolCloser(r, func(string) { close(closed) })
	lease := takeLease(t, r, "")
	r.mu.Lock()
	entry := r.entries["corpus_vn_20990103"]
	entry.opening = true
	entry.openingDone = make(chan struct{})
	r.mu.Unlock()
	r.Close()
	finished := make(chan struct{})
	go func() { lease.Done(); close(finished) }()
	select {
	case <-finished:
		t.Error("final lease returned before pending open completed")
	case <-time.After(20 * time.Millisecond):
	}
	r.mu.Lock()
	entry.opening = false
	close(entry.openingDone)
	entry.openingDone = nil
	r.mu.Unlock()
	<-finished
	select {
	case <-closed:
	default:
		t.Error("final lease left pool open after pending open completed")
	}
}
