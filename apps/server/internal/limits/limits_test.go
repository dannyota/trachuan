package limits

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAllow_SearchSixtyPerMinute(t *testing.T) {
	now := time.Unix(0, 0)
	l := New(func() time.Time { return now })
	for i := 0; i < 60; i++ {
		if ok, _ := l.Allow(1, Search); !ok {
			t.Fatalf("request %d denied", i)
		}
	}
	if ok, retry := l.Allow(1, Search); ok || retry != time.Second {
		t.Fatalf("got %v, %v", ok, retry)
	}
	if ok, _ := l.Allow(2, Search); !ok {
		t.Fatal("another account denied")
	}
}

func TestAllow_ReadBucketSeparate(t *testing.T) {
	l := New(func() time.Time { return time.Unix(0, 0) })
	for i := 0; i < 60; i++ {
		l.Allow(1, Search)
	}
	for i := 0; i < 120; i++ {
		if ok, _ := l.Allow(1, Read); !ok {
			t.Fatalf("read %d denied", i)
		}
	}
	if ok, retry := l.Allow(1, Read); ok || retry != 500*time.Millisecond {
		t.Fatalf("got %v, %v", ok, retry)
	}
}

func TestAllow_RefillOverTime(t *testing.T) {
	now := time.Unix(0, 0)
	l := New(func() time.Time { return now })
	for i := 0; i < 60; i++ {
		l.Allow(1, Search)
	}
	now = now.Add(250 * time.Millisecond)
	if ok, retry := l.Allow(1, Search); ok || retry != 750*time.Millisecond {
		t.Fatalf("got %v, %v", ok, retry)
	}
	now = now.Add(750 * time.Millisecond)
	if ok, retry := l.Allow(1, Search); !ok || retry != 0 {
		t.Fatalf("got %v, %v", ok, retry)
	}
	now = now.Add(2 * time.Minute)
	for i := 0; i < 60; i++ {
		if ok, _ := l.Allow(1, Search); !ok {
			t.Fatalf("refilled request %d denied", i)
		}
	}
	if ok, _ := l.Allow(1, Search); ok {
		t.Fatal("refill exceeded capacity")
	}
}

func TestAllow_ConcurrentSafe(t *testing.T) {
	l := New(func() time.Time { return time.Unix(0, 0) })
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _ := l.Allow(1, Search); ok {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := allowed.Load(); got != 60 {
		t.Fatalf("allowed %d", got)
	}
}

func TestAllow_IdleAccountsEvicted(t *testing.T) {
	now := time.Unix(0, 0)
	l := New(func() time.Time { return now })
	l.Allow(1, Search)
	now = now.Add(10 * time.Minute)
	l.Allow(2, Search)
	if len(l.buckets) != 2 {
		t.Fatalf("evicted at ten minutes: %d", len(l.buckets))
	}
	now = now.Add(time.Nanosecond)
	l.Allow(3, Search)
	if len(l.buckets) != 2 {
		t.Fatalf("idle bucket retained: %d", len(l.buckets))
	}
}

func TestAllow_CapacityPreservesDepletedBuckets(t *testing.T) {
	for _, tc := range []struct {
		name string
		wait time.Duration
	}{
		{name: "full bucket", wait: time.Second},
		{name: "idle bucket", wait: 10*time.Minute + time.Nanosecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Unix(0, 0)
			l := New(func() time.Time { return now })
			for i := int64(0); i < 100000; i++ {
				l.Allow(i, Search)
			}
			for i := 0; i < 59; i++ {
				l.Allow(0, Search)
			}
			if ok, retry := l.Allow(100000, Search); ok || retry != time.Second {
				t.Fatalf("new key at capacity: got %v, %v", ok, retry)
			}
			if ok, retry := l.Allow(0, Search); ok || retry != time.Second {
				t.Fatalf("depleted key after denial: got %v, %v", ok, retry)
			}
			now = now.Add(tc.wait)
			if ok, retry := l.Allow(100000, Search); !ok || retry != 0 {
				t.Fatalf("new key after wait: got %v, %v", ok, retry)
			}
			if tc.wait == time.Second {
				if ok, _ := l.Allow(0, Search); !ok {
					t.Fatal("depleted key did not refill")
				}
				if ok, retry := l.Allow(0, Search); ok || retry != time.Second {
					t.Fatalf("depleted key was reset: got %v, %v", ok, retry)
				}
			}
		})
	}
}
