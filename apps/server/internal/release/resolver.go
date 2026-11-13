package release

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
	"golang.org/x/sync/singleflight"
)

var ErrExpired = errors.New("release expired")

type Store interface {
	GetActiveRelease(context.Context, string) (string, error)
}

type Resolver struct {
	Auth                Store
	Open                func(ctx context.Context, dbName string) (*retrieval.Engine, func(), error)
	Clock               func() time.Time
	URLTemplate         string
	EmbedderFor         func(queryRecipeHash string, expected embedding.Recipe) retrieval.Embedder
	Options             retrieval.Options
	OnPostings          func(string, retrieval.PostingsOutcome)
	AllowTestRecipe     bool
	OnTestRecipeRefused func()
	QueryTimeout        time.Duration

	OnActivate func(release string, generation uint64)

	mu          sync.Mutex
	generation  uint64
	observation chan struct{}
	entries     map[string]*cacheEntry
	cleanups    map[*cacheEntry]chan struct{}
	active      map[string]string
	workCtx     context.Context
	cancelWork  context.CancelFunc
	flight      singleflight.Group
	closed      bool
}

func (r *Resolver) Resolve(ctx context.Context, country, rel string) (*Lease, error) {
	if country != "vn" {
		return nil, ErrExpired
	}
	name, _, _, err := r.resolveName(ctx, country, rel)
	if err != nil {
		return nil, err
	}
	for {
		result := r.flight.DoChan(name, func() (any, error) {
			workCtx, cancel, err := r.workContext()
			if err != nil {
				return nil, err
			}
			defer cancel()
			return r.load(workCtx, country, name)
		})
		var opened singleflight.Result
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case opened = <-result:
		}
		current, currentRelease, generation, err := r.resolveName(ctx, country, rel)
		if err != nil {
			return nil, err
		}
		if current != name {
			name = current
			continue
		}
		if opened.Err != nil {
			return nil, opened.Err
		}
		entry := opened.Val.(*cacheEntry)
		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			return nil, ErrExpired
		}
		if entry.retired || r.entries[name] != entry {
			r.mu.Unlock()
			continue
		}
		entry.leases++
		lease := &Lease{Engine: entry.engine, Release: currentRelease, Generation: generation, done: func() { r.finish(entry) }}
		r.mu.Unlock()
		return lease, nil
	}
}

func (r *Resolver) workContext() (context.Context, context.CancelFunc, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, nil, ErrExpired
	}
	if r.workCtx == nil {
		r.workCtx, r.cancelWork = context.WithCancel(context.Background())
	}
	ctx, cancel := context.WithTimeout(r.workCtx, 10*time.Second)
	return ctx, cancel, nil
}

func validDBName(country, name string) bool {
	return regexp.MustCompile(`^corpus_[a-z]{2}_[0-9]{8}(_[0-9]+)?$`).MatchString(name) && strings.HasPrefix(name, "corpus_"+country+"_")
}

func (r *Resolver) Close() {
	r.mu.Lock()
	r.closed = true
	cancel := r.cancelWork
	for _, entry := range r.entries {
		entry.retired = true
		if entry.engine != nil {
			entry.engine.Retire()
		}
		if !entry.opening && entry.leases == 0 {
			r.cleanupLocked(entry)
		}
	}
	r.entries = nil
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	for {
		r.mu.Lock()
		done := make([]chan struct{}, 0, len(r.cleanups))
		for _, cleanup := range r.cleanups {
			done = append(done, cleanup)
		}
		r.mu.Unlock()
		if len(done) == 0 {
			return
		}
		for _, cleanup := range done {
			<-cleanup
		}
	}
}
