package loadrun

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestSchedule_OpenLoop(t *testing.T) {
	started := make(chan struct{}, 20)
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		schedule(context.Background(), 20*time.Millisecond, 1000, func(int) { started <- struct{}{}; <-release })
		close(done)
	}()
	timer := time.NewTimer(80 * time.Millisecond)
	defer timer.Stop()
	for i := 0; i < 15; i++ {
		select {
		case <-started:
		case <-timer.C:
			close(release)
			<-done
			t.Fatalf("only %d requests started before responses released", i)
		}
	}
	close(release)
	<-done
}
func TestQueries_Exhaustion(t *testing.T) {
	q, err := newQueries([]string{"one", "two"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := q.next(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := q.next(); err == nil {
		t.Fatal("expected exhaustion")
	}
	if _, err := newQueries([]string{"one", "one"}); err == nil {
		t.Fatal("expected duplicate error")
	}
}
func TestRetry_TimingAndLimit(t *testing.T) {
	var waits []time.Duration
	attempts := 0
	retries := retryBusy(func() (bool, time.Duration) { attempts++; return true, 3 * time.Second }, func(d time.Duration) { waits = append(waits, d) })
	if attempts != 4 || retries != 3 || len(waits) != 3 || waits[0] != 3*time.Second || waits[1] != 3*time.Second || waits[2] != 4*time.Second {
		t.Fatalf("attempts %d waits %v", attempts, waits)
	}
}
func TestAbort_Fraction(t *testing.T) {
	count := 0
	addresses := map[int]bool{}
	for i := 0; i < 1000; i++ {
		if abortRequest(i, 42) {
			count++
			addresses[i%40] = true
		}
	}
	if len(addresses) < 10 {
		t.Fatalf("only %d addresses selected", len(addresses))
	}
	if count != 50 {
		t.Fatalf("aborts %d", count)
	}
}
func TestAddresses_Count(t *testing.T) {
	for _, n := range []int{16, 40, 50} {
		seen := map[string]bool{}
		for i := 0; i < n*2; i++ {
			seen[address(i, n)] = true
		}
		if len(seen) != n {
			t.Fatalf("got %d addresses", len(seen))
		}
	}
}
func TestPercentile_NearestRank(t *testing.T) {
	if got := percentile([]float64{4, 1, 3, 2}, 95); got != 4 {
		t.Fatalf("got %v", got)
	}
	if got := percentile([]float64{4, 1, 3, 2}, 50); got != 2 {
		t.Fatalf("got %v", got)
	}
}

func TestSchedule_UntilCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var started atomic.Int32
	schedule(ctx, 0, 10000, func(int) {
		if started.Add(1) >= 2 {
			cancel()
		}
	})
	if started.Load() < 2 {
		t.Fatalf("started %d", started.Load())
	}
}
