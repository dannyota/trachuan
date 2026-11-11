package release

import (
	"context"
	"errors"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

func TestResolver_OnActivateOrdersSwitches(t *testing.T) {
	r, s, _ := resolverFixture(t)
	var observed []string
	r.OnActivate = func(release string) { observed = append(observed, release) }
	first := takeLease(t, r, "")
	takeLease(t, r, "20990103").Done()
	s.active = "corpus_vn_20990104"
	second := takeLease(t, r, "")
	first.Done()
	if !slices.Equal(observed, []string{"20990103", "20990104"}) {
		t.Fatalf("notifications after old lease ended=%v", observed)
	}
	s.active = "corpus_vn_20990103"
	third := takeLease(t, r, "")
	second.Done()
	third.Done()
	r.Close()
	if !slices.Equal(observed, []string{"20990103", "20990104", "20990103"}) {
		t.Fatalf("notifications=%v", observed)
	}
}

func TestResolver_OnActivateNotifiesFailedOpen(t *testing.T) {
	r, s, engine := resolverFixture(t)
	failure := errors.New("engine unavailable")
	r.Open = func(_ context.Context, name string) (*retrieval.Engine, func(), error) {
		if name == "corpus_vn_20990104" {
			return nil, nil, failure
		}
		return engine, nil, nil
	}
	var observed []string
	r.OnActivate = func(release string) { observed = append(observed, release) }
	first := takeLease(t, r, "")
	s.active = "corpus_vn_20990104"
	for range 2 {
		if err := resolveAndFinish(context.Background(), r, ""); !errors.Is(err, failure) {
			t.Fatalf("open error=%v", err)
		}
	}
	first.Done()
	if !slices.Equal(observed, []string{"20990103", "20990104"}) {
		t.Fatalf("notifications=%v", observed)
	}
}

func TestResolver_OnActivateNotifiesEmptyPointer(t *testing.T) {
	for _, initiallyActive := range []bool{false, true} {
		name := "initial_empty"
		if initiallyActive {
			name = "clear_active"
		}
		t.Run(name, func(t *testing.T) {
			r, s, _ := resolverFixture(t)
			var observed []string
			r.OnActivate = func(release string) { observed = append(observed, release) }
			if initiallyActive {
				takeLease(t, r, "")
			}
			s.active = ""
			for range 2 {
				if err := resolveAndFinish(context.Background(), r, ""); !errors.Is(err, ErrExpired) {
					t.Fatalf("empty pointer error=%v", err)
				}
			}
			want := []string{""}
			if initiallyActive {
				want = []string{"20990103", ""}
			}
			if !slices.Equal(observed, want) {
				t.Fatalf("notifications=%v want=%v", observed, want)
			}
		})
	}
}

func TestResolver_OnActivateCompletesBeforeNewLease(t *testing.T) {
	r, s, _ := resolverFixture(t)
	entered, proceed := make(chan struct{}), make(chan struct{})
	var notified atomic.Bool
	r.OnActivate = func(release string) {
		if release == "20990104" {
			close(entered)
			<-proceed
			notified.Store(true)
		}
	}
	takeLease(t, r, "")
	s.active = "corpus_vn_20990104"
	result := make(chan error, 1)
	go func() {
		lease, err := r.Resolve(context.Background(), "vn", "")
		if lease != nil {
			defer lease.Done()
			if !notified.Load() {
				err = errors.New("new lease arrived before notification completed")
			}
		}
		result <- err
	}()
	select {
	case <-entered:
	case err := <-result:
		t.Fatalf("resolve completed before the B notification: %v", err)
	}
	select {
	case err := <-result:
		close(proceed)
		t.Fatalf("resolve completed during the B notification: %v", err)
	default:
	}
	close(proceed)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

type activationStore struct {
	mu     sync.Mutex
	active string
	reads  []string
}

func (s *activationStore) GetActiveRelease(context.Context, string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reads = append(s.reads, s.active)
	return s.active, nil
}

func TestResolver_OnActivateConcurrentObservationOrder(t *testing.T) {
	r, _, _ := resolverFixture(t)
	s := &activationStore{active: "corpus_vn_20990103"}
	r.Auth = s
	var mu sync.Mutex
	var observed []string
	var inCallback atomic.Int32
	r.OnActivate = func(release string) {
		if inCallback.Add(1) != 1 {
			t.Error("concurrent activation callbacks")
		}
		defer inCallback.Add(-1)
		runtime.Gosched()
		mu.Lock()
		observed = append(observed, release)
		mu.Unlock()
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Go(func() {
			<-start
			s.mu.Lock()
			s.active = "corpus_vn_20990103"
			if i%2 != 0 {
				s.active = "corpus_vn_20990104"
			}
			s.mu.Unlock()
			if err := resolveAndFinish(context.Background(), r, ""); err != nil && !errors.Is(err, ErrExpired) {
				t.Error(err)
			}
		})
	}
	close(start)
	wg.Wait()
	var want []string
	for _, name := range s.reads {
		release := "20990103"
		if name == "corpus_vn_20990104" {
			release = "20990104"
		}
		if len(want) == 0 || want[len(want)-1] != release {
			want = append(want, release)
		}
	}
	if !slices.Equal(observed, want) {
		t.Fatalf("notifications=%v pointer observations=%v", observed, want)
	}
}
