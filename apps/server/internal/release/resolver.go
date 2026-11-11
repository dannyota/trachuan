// Package release resolves the active corpus release.
package release

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"danny.vn/trachuan/internal/retrieval"
	"golang.org/x/sync/singleflight"
)

// ErrExpired means a requested release is not active.
var ErrExpired = errors.New("release expired")

// Store is the resolver's auth database view. GetActiveRelease returns active_db
// from corpus_release for country, or an empty string when no row exists.
// Adapters should translate a missing active row into an empty name, not an error.
type Store interface {
	GetActiveRelease(context.Context, string) (string, error)
}

// Resolver caches engines and pools by trusted corpus database name.
// Configure URLTemplate and EmbedderFor. Open overrides the opener for tests.
// URLTemplate contains exactly one {database} placeholder in its database name.
// Injected openers honor their context and return a closer for that exact pool,
// including when opening fails. They must not close pools by database name.
// Configure fields before the first Resolve and do not modify them concurrently.
type Resolver struct {
	Auth        Store
	Open        func(ctx context.Context, dbName string) (*retrieval.Engine, func(), error)
	Clock       func() time.Time
	URLTemplate string
	EmbedderFor func(queryRecipeHash string) retrieval.Embedder
	Options     retrieval.Options

	// OnActivate reports the first pointer observation and each change, including
	// an empty pointer. Calls run under the resolver lock before lending leases.
	// The callback must not call resolver methods.
	OnActivate func(release string)

	mu         sync.Mutex
	entries    map[string]*cacheEntry
	active     map[string]string
	workCtx    context.Context
	cancelWork context.CancelFunc
	flight     singleflight.Group
	closed     bool
}

// Resolve leases the active engine for one request. A nonempty rel must match
// the active pointer. The caller must call Done when the request ends.
func (r *Resolver) Resolve(ctx context.Context, country, rel string) (*Lease, error) {
	if country != "vn" {
		return nil, ErrExpired
	}
	name, release, err := r.resolveName(ctx, country, rel)
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
		// Opening can outlive an activation even when no other request observes it.
		current, currentRelease, err := r.resolveName(ctx, country, rel)
		if err != nil {
			return nil, err
		}
		if current != name {
			name, release = current, currentRelease
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
			// An activation and rollback can retire this entry while preserving
			// the same database name. Lease only the replacement entry.
			continue
		}
		entry.leases++
		lease := &Lease{Engine: entry.engine, Release: release, done: func() { r.finish(entry) }}
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

func (r *Resolver) resolveName(ctx context.Context, country, rel string) (string, string, error) {
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	if r.Auth == nil || r.Clock == nil {
		return "", "", errors.New("missing release resolver dependencies")
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return "", "", ErrExpired
	}
	active, err := r.Auth.GetActiveRelease(ctx, country)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		r.mu.Unlock()
		return "", "", fmt.Errorf("get active release: %w", err)
	}
	if active != "" && !validDBName(country, active) {
		r.mu.Unlock()
		return "", "", errors.New("invalid active database name")
	}
	if r.active == nil {
		r.active = make(map[string]string)
	}
	previous, observed := r.active[country]
	r.active[country] = active
	var retired []*cacheEntry
	for name, entry := range r.entries {
		if entry.country == country && name != active {
			delete(r.entries, name)
			entry.retired = true
			if !entry.opening && entry.leases == 0 {
				retired = append(retired, entry)
			}
		}
	}
	if r.OnActivate != nil && (!observed || previous != active) {
		r.OnActivate(strings.TrimPrefix(active, "corpus_"+country+"_"))
	}
	allowed := active != "" && (rel == "" || active == "corpus_"+country+"_"+rel)
	r.mu.Unlock()
	for _, entry := range retired {
		entry.close()
	}
	if !allowed {
		return "", "", ErrExpired
	}
	return active, strings.TrimPrefix(active, "corpus_"+country+"_"), nil
}

func validDBName(country, name string) bool {
	return regexp.MustCompile(`^corpus_[a-z]{2}_[0-9]{8}(_[0-9]+)?$`).MatchString(name) && strings.HasPrefix(name, "corpus_"+country+"_")
}

// Close cancels shared work and prevents future resolves. Pools close after
// their last lease ends and any pending opener finishes.
func (r *Resolver) Close() {
	r.mu.Lock()
	r.closed = true
	cancel := r.cancelWork
	var entries []*cacheEntry
	for _, entry := range r.entries {
		entry.retired = true
		if !entry.opening && entry.leases == 0 {
			entries = append(entries, entry)
		}
	}
	r.entries = nil
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	for _, entry := range entries {
		entry.close()
	}
}
