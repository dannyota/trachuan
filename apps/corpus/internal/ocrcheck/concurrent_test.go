package ocrcheck

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

type concurrentClient struct {
	Client
	tokens              int64
	response            Response
	call                func(context.Context, Request) (Response, error)
	count               func(context.Context) error
	mu                  sync.Mutex
	calls, active, peak int
}

func (c *concurrentClient) CountTokens(ctx context.Context, _ Request) (int64, error) {
	if c.count != nil {
		if err := c.count(ctx); err != nil {
			return 0, err
		}
	}
	return c.tokens, nil
}
func (c *concurrentClient) Direct(ctx context.Context, r Request) (Response, error) {
	c.mu.Lock()
	c.calls++
	c.active++
	if c.active > c.peak {
		c.peak = c.active
	}
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.active--; c.mu.Unlock() }()
	if c.call != nil {
		return c.call(ctx, r)
	}
	return c.response, nil
}

type concurrentRenderer struct{ paths []string }

func (concurrentRenderer) Preflight(context.Context) error { return nil }
func (f concurrentRenderer) RenderJPEGDPI(context.Context, []byte, int, int) ([]string, func(), error) {
	return f.paths, func() {}, nil
}
func concurrentFixture(t *testing.T, n int) (*Runner, *memoryRepository, *concurrentClient, []Document) {
	t.Helper()
	r, repo, old, docs, f := jpegFixture(t)
	r.JPEGRenderer = concurrentRenderer{paths: f.paths}
	c := &concurrentClient{Client: old, tokens: 100, response: old.Response}
	r.Client = c
	for i := 1; i < n; i++ {
		d := docs[0]
		d.Entry.ID = int64(i + 1)
		docs = append(docs, d)
	}
	return r, repo, c, docs
}

func TestConcurrent_EightInFlight(t *testing.T) {
	for _, version := range []int{3, 4} {
		r, _, c, docs := concurrentFixture(t, 24)
		release := make(chan struct{})
		var entered int
		var mu sync.Mutex
		c.call = func(ctx context.Context, _ Request) (Response, error) {
			mu.Lock()
			entered++
			if entered == 8 {
				close(release)
			}
			mu.Unlock()
			select {
			case <-release:
				return c.response, nil
			case <-ctx.Done():
				return Response{}, ctx.Err()
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		o := jpegOptions()
		o.CheckVersion = version
		s, err := r.Run(ctx, docs, o)
		cancel()
		if err != nil || s.Complete != 24 || c.peak != 8 {
			t.Fatalf("version=%d complete=%d peak=%d err=%v", version, s.Complete, c.peak, err)
		}
	}
}

func TestConcurrent_AtomicBudget(t *testing.T) {
	r, repo, c, docs := concurrentFixture(t, 20)
	c.response.Usage = Usage{Input: 100, Output: 32000}
	all := make(chan struct{})
	var mu sync.Mutex
	counted := 0
	c.count = func(ctx context.Context) error {
		mu.Lock()
		counted++
		if counted == 8 {
			close(all)
		}
		mu.Unlock()
		select {
		case <-all:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	o := jpegOptions()
	o.MaxUSD = .960600
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	s, err := r.Run(ctx, docs, o)
	if !errors.Is(err, ErrBudget) || c.calls != 3 || s.Complete != 3 || len(repo.attempts) != 3 {
		t.Fatalf("calls=%d attempts=%d summary=%+v err=%v", c.calls, len(repo.attempts), s, err)
	}
}

func TestConcurrent_UncertainDrainsInFlight(t *testing.T) {
	r, repo, c, docs := concurrentFixture(t, 20)
	all := make(chan struct{})
	recorded := make(chan struct{})
	var mu sync.Mutex
	entered := 0
	c.call = func(ctx context.Context, _ Request) (Response, error) {
		mu.Lock()
		entered++
		n := entered
		if n == 8 {
			close(all)
		}
		mu.Unlock()
		select {
		case <-all:
		case <-ctx.Done():
			return Response{}, ctx.Err()
		}
		if n == 1 {
			return Response{}, context.DeadlineExceeded
		}
		select {
		case <-recorded:
			return c.response, nil
		case <-ctx.Done():
			return Response{}, ctx.Err()
		}
	}
	r.Repository = &observedRepository{memoryRepository: repo, uncertain: recorded}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	s, err := r.Run(ctx, docs, jpegOptions())
	if err == nil || c.calls != 8 || s.Complete != 7 || s.Pending != 1 || len(repo.attempts) != 8 {
		t.Fatalf("calls=%d summary=%+v err=%v", c.calls, s, err)
	}
}

type observedRepository struct {
	*memoryRepository
	uncertain chan struct{}
	crash     bool
}

func (s *observedRepository) Finish(ctx context.Context, a Attempt, e store.OcrText, u Usage, result *ocrstore.CheckResult, reason string) error {
	if s.crash {
		return errors.New("simulated crash before terminal write")
	}
	err := s.memoryRepository.Finish(ctx, a, e, u, result, reason)
	if err == nil && !u.Final && u.ReservationMicroUSD > 0 {
		close(s.uncertain)
	}
	return err
}

func TestConcurrent_RealCostReleasesReservation(t *testing.T) {
	r, repo, c, docs := concurrentFixture(t, 4)
	o := jpegOptions()
	o.LimitDocs = 3
	o.MaxUSD = .960600
	s, err := r.Run(context.Background(), docs, o)
	if err != nil || s.Complete != 3 || s.CostMicroUSD != 3600 {
		t.Fatalf("summary=%+v err=%v", s, err)
	}
	o.LimitDocs = 0
	s, err = r.Run(context.Background(), docs, o)
	if err != nil || s.Complete != 1 || c.calls != 4 || s.CostMicroUSD != 4800 || len(repo.attempts) != 4 {
		t.Fatalf("summary=%+v calls=%d err=%v", s, c.calls, err)
	}
}

func TestConcurrent_CrashRetainsPreparing(t *testing.T) {
	r, repo, c, docs := concurrentFixture(t, 8)
	r.Repository = &observedRepository{memoryRepository: repo, crash: true}
	if _, err := r.Run(context.Background(), docs, jpegOptions()); err == nil {
		t.Fatal("simulated crash succeeded")
	}
	calls := c.calls
	for _, a := range repo.attempts {
		if a.State != "preparing" || a.Usage.charge() != 320200 {
			t.Fatalf("attempt=%+v", a)
		}
	}
	r.Repository = repo
	s, err := r.Run(context.Background(), docs, jpegOptions())
	if err != nil || s.CostMicroUSD != int64(calls)*320200 || c.calls != calls || s.Pending != calls {
		t.Fatalf("summary=%+v calls=%d err=%v", s, c.calls, err)
	}
	r.Now = func() time.Time { return time.Date(2099, 1, 1, 0, 11, 0, 0, time.UTC) }
	if _, err = r.Run(context.Background(), docs, jpegOptions()); err == nil || c.calls != calls {
		t.Fatal("stale preparing rows did not block")
	}
}

func TestConcurrent_DryRunAndRetryOverride(t *testing.T) {
	r, repo, c, docs := concurrentFixture(t, 4)
	docs = append(docs, docs[0])
	o := jpegOptions()
	o.DryRun = true
	o.LimitDocs = 3
	s, err := r.Run(context.Background(), docs, o)
	if err != nil || s.Estimated != 3 || s.EstimateMicroUSD != 960600 || c.calls != 0 || len(repo.attempts) != 0 {
		t.Fatalf("summary=%+v err=%v", s, err)
	}
	o.DryRun = false
	c.response.Text = `{"same_document":true,"pages":0,"items":[]}`
	for range 2 {
		if _, err = r.Run(context.Background(), docs, o); err != nil {
			t.Fatal(err)
		}
	}
	s, err = r.Run(context.Background(), docs, o)
	if err != nil || s.SkippedRetryLimit != 3 || s.Failed != 1 || c.calls != 7 {
		t.Fatalf("summary=%+v calls=%d err=%v", s, c.calls, err)
	}
	o.RetryFailed = true
	c.response.Text = `{"same_document":true,"pages":10,"items":[]}`
	s, err = r.Run(context.Background(), docs, o)
	if err != nil || s.Complete != 3 || c.calls != 10 {
		t.Fatalf("summary=%+v calls=%d err=%v", s, c.calls, err)
	}
}
