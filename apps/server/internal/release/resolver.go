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
// Configure either URLTemplate and Embedder, or Open.
// URLTemplate contains exactly one {database} placeholder in its database name.
// Injected openers supply one pool per name and CloseDatabase to retire it.
// Configure fields before the first Resolve and do not modify them concurrently.
type Resolver struct {
	Auth          Store
	Open          func(dbName string) (*retrieval.Engine, error)
	Clock         func() time.Time
	URLTemplate   string
	Embedder      retrieval.Embedder
	Options       retrieval.Options
	CloseDatabase func(dbName string)

	mu      sync.Mutex
	entries map[string]*cacheEntry
	active  map[string]string
	flight  singleflight.Group
	closed  bool
}

// Resolve returns an engine and its release identifier, resolving an empty rel
// to the active pointer. Every call checks the auth store before using the cache.
func (r *Resolver) Resolve(ctx context.Context, country, rel string) (*retrieval.Engine, string, error) {
	if country != "vn" {
		return nil, "", ErrExpired
	}
	name, release, err := r.resolveName(ctx, country, rel)
	if err != nil {
		return nil, "", err
	}

	for {
		result := r.flight.DoChan(name, func() (any, error) {
			return nil, r.load(ctx, country, name)
		})
		var openErr error
		select {
		case <-ctx.Done():
			return nil, "", ctx.Err()
		case opened := <-result:
			openErr = opened.Err
		}
		r.mu.Lock()
		entry := r.entries[name]
		if r.closed || r.active[country] != name {
			r.mu.Unlock()
			return nil, "", ErrExpired
		}
		var engine *retrieval.Engine
		if entry != nil {
			engine = entry.engine
		}
		r.mu.Unlock()
		if openErr != nil {
			return nil, "", openErr
		}
		if engine != nil {
			return engine, release, nil
		}
		// An activation can evict and reauthorize this name before the caller
		// consumes the open result. Retry against the current cache entry.
	}
}

func (r *Resolver) resolveName(ctx context.Context, country, rel string) (string, string, error) {
	if r.Auth == nil || r.Clock == nil {
		return "", "", errors.New("missing release resolver dependencies")
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return "", "", errors.New("release resolver closed")
	}
	active, err := r.Auth.GetActiveRelease(ctx, country)
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
	r.active[country] = active
	var retired []*cacheEntry
	for name, entry := range r.entries {
		if entry.country == country && name != active {
			delete(r.entries, name)
			if !entry.opening {
				retired = append(retired, entry)
			}
		}
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

// Close releases cached pools and prevents future resolves.
// A pending opener closes its pool as soon as it finishes.
func (r *Resolver) Close() {
	r.mu.Lock()
	r.closed = true
	var entries []*cacheEntry
	for _, entry := range r.entries {
		if !entry.opening {
			entries = append(entries, entry)
		}
	}
	r.entries = nil
	r.mu.Unlock()
	for _, entry := range entries {
		entry.close()
	}
}
