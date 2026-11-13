package retrieval

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"danny.vn/trachuan/internal/retrieval/lexical"
)

type PostingsOutcome struct {
	Status             string
	Reason             string
	LoadSeconds        float64
	Bytes              int64
	EstimatedPeakBytes int64
}

type LexicalSources struct {
	SQL      uint64
	Postings uint64
	Reused   uint64
}

type sparseLifecycle struct {
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	started  bool
	closed   bool
	retired  bool
	index    atomic.Pointer[sparsePostings]
	sql      atomic.Uint64
	postings atomic.Uint64
	reused   atomic.Uint64
}

func (s *sparseLifecycle) init() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
}

func (e *Engine) StartSparsePostings(report func(PostingsOutcome)) {
	s := &e.sparse
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started || s.closed || s.retired {
		return
	}
	serving := e.options.Profile == "serving"
	if serving && (e.db == nil || s.ctx == nil) {
		return
	}
	s.started = true
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if !serving {
			if report != nil {
				report(PostingsOutcome{Status: "disabled", Reason: "profile"})
			}
			return
		}
		start := time.Now()
		index, outcome := e.loadPostings(s.ctx)
		if index != nil && e.publishPostings(index) {
			outcome.Status = "ready"
		} else {
			outcome.Status = "disabled"
			if s.ctx.Err() != nil {
				outcome.Reason = "load canceled"
			}
			if index != nil {
				outcome.Reason = "engine retired or closed"
			}
		}
		outcome.LoadSeconds = time.Since(start).Seconds()
		if report != nil {
			report(outcome)
		}
	}()
}

func (e *Engine) publishPostings(index *sparsePostings) bool {
	s := &e.sparse
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.retired {
		return false
	}
	s.index.Store(index)
	return true
}

func (e *Engine) Retire() {
	s := &e.sparse
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retired = true
	if s.cancel != nil {
		s.cancel()
	}
}

func (e *Engine) Close() {
	s := &e.sparse
	s.mu.Lock()
	s.closed = true
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
	s.index.Store(nil)
}

func (e *Engine) SparsePostingsReady() bool { return e.sparse.index.Load() != nil }

func (e *Engine) LexicalSources() LexicalSources {
	return LexicalSources{SQL: e.sparse.sql.Load(), Postings: e.sparse.postings.Load(), Reused: e.sparse.reused.Load()}
}

func (e *Engine) postingsFor(query lexical.Vector) *sparsePostings {
	for _, entry := range query {
		if entry.Value != 1 {
			return nil
		}
	}
	return e.sparse.index.Load()
}
