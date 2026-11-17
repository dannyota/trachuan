package release

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"danny.vn/trachuan/internal/frameworks/serve"
)

func TestResolver_FrameworksLeaseLifecycle(t *testing.T) {
	s := &fakeStore{active: "frameworks_20990103"}
	var opened, closed atomic.Int32
	r := &Resolver{Auth: s, Clock: time.Now, Frameworks: &serve.Options{}, OpenFrameworks: func(context.Context, string) (*serve.Engine, func(), error) {
		opened.Add(1)
		return &serve.Engine{}, func() { closed.Add(1) }, nil
	}}
	t.Cleanup(r.Close)
	<-r.refreshFrameworks(t.Context())
	first, err := r.Resolve(t.Context(), "vn", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.Resolve(t.Context(), "vn", "20990103")
	if err != nil {
		t.Fatal(err)
	}
	if first.Frameworks == nil || first.Frameworks != second.Frameworks || first.Engine != nil || first.Release != "20990103" || opened.Load() != 1 {
		t.Fatal("frameworks leases did not share their engine")
	}
	second.Done()
	s.active = "frameworks_20990104"
	<-r.refreshFrameworks(t.Context())
	third, err := r.Resolve(t.Context(), "vn", "")
	if err != nil {
		t.Fatal(err)
	}
	if closed.Load() != 0 {
		t.Fatal("closed pool with an outstanding lease")
	}
	first.Done()
	waitResolverCleanup(r)
	if closed.Load() != 1 {
		t.Fatal("retired pool did not close")
	}
	r.Close()
	third.Done()
	if closed.Load() != 2 {
		t.Fatal("active pool did not close after final lease")
	}
}

func TestResolver_FrameworksRejectsLawPointerAndExpiredPin(t *testing.T) {
	for _, name := range []string{"corpus_vn_20990103", "frameworks_20990103/other", "frameworks_20990103;DROP"} {
		r := &Resolver{Auth: &fakeStore{active: name}, Clock: time.Now, Frameworks: &serve.Options{}}
		if _, err := r.Resolve(t.Context(), "vn", ""); err == nil {
			t.Fatalf("accepted %q", name)
		}
		r.Close()
	}
	r := &Resolver{Auth: &fakeStore{active: "frameworks_20990103"}, Clock: time.Now, Frameworks: &serve.Options{}}
	t.Cleanup(r.Close)
	if _, err := r.Resolve(t.Context(), "vn", "20990102"); !errors.Is(err, ErrExpired) {
		t.Fatalf("pin error=%v", err)
	}
}
