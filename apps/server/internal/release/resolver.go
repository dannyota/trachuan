// Package release resolves active and retained corpus releases.
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

// ErrExpired means a requested release is absent, dropped, or past retention.
var ErrExpired = errors.New("release expired")

// ErrIncompatible means a retained release is readable but cannot be searched.
var ErrIncompatible = errors.New("release incompatible")

// RetainedRelease contains the retirement fields used to authorize a release.
type RetainedRelease struct {
	DBName      string
	RetainUntil time.Time
	DroppedAt   *time.Time
}

// Store is the resolver's auth database view. GetActiveRelease returns active_db
// from corpus_release for country, or an empty string when no row exists.
// ListRetainedReleases returns retired_release rows shaped as db_name,
// retain_until, dropped_at. The resolver also filters expired and dropped rows.
// Adapters should translate a missing active row into an empty name, not an error.
type Store interface {
	GetActiveRelease(context.Context, string) (string, error)
	ListRetainedReleases(context.Context, string) ([]RetainedRelease, error)
}

// Resolver caches engines and pools by trusted corpus database name.
// Configure either URLTemplate and Embedder, or both Open and OpenRead.
// URLTemplate contains exactly one {database} placeholder in its database name.
// Injected openers share one pool per name and supply CloseDatabase to retire it.
// Configure fields before the first Resolve and do not modify them concurrently.
type Resolver struct {
	Auth          Store
	Open          func(dbName string) (*retrieval.Engine, error)
	OpenRead      func(dbName string) (*retrieval.Engine, error)
	Clock         func() time.Time
	URLTemplate   string
	Embedder      retrieval.Embedder
	Options       retrieval.Options
	CloseDatabase func(dbName string)

	mu      sync.Mutex
	entries map[string]*cacheEntry
	live    map[string]map[string]bool
	flight  singleflight.Group
	closed  bool
}

// Resolve returns an engine and its release identifier, resolving an empty rel
// to the active pointer. Every call checks the auth store before using the cache.
func (r *Resolver) Resolve(ctx context.Context, country, rel string, forSearch bool) (*retrieval.Engine, string, error) {
	if country != "vn" {
		return nil, "", ErrExpired
	}
	name, release, err := r.resolveName(ctx, country, rel)
	if err != nil {
		return nil, "", err
	}

	for {
		result := r.flight.DoChan(name, func() (any, error) {
			_, openErr := r.load(ctx, country, name, forSearch)
			return openAttempt{search: forSearch, err: openErr}, nil
		})
		var attempt openAttempt
		select {
		case <-ctx.Done():
			return nil, "", ctx.Err()
		case opened := <-result:
			attempt = opened.Val.(openAttempt)
		}
		r.mu.Lock()
		entry := r.entries[name]
		if r.closed || !r.live[country][name] {
			r.mu.Unlock()
			return nil, "", ErrExpired
		}
		var engine *retrieval.Engine
		if entry != nil {
			engine = entry.search
			if engine == nil && !forSearch {
				engine = entry.read
			}
		}
		r.mu.Unlock()
		if attempt.search == forSearch && attempt.err != nil {
			return nil, "", attempt.err
		}
		if engine != nil {
			return engine, release, nil
		}
		// A caller needing the other capability may win the single flight. Retry
		// only the missing capability; never pass a search-only error to a read.
		// An activation may also evict and reauthorize the name before this caller
		// consumes the result. A successful attempt then needs a fresh cache entry.
	}
}

type openAttempt struct {
	search bool
	err    error
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
	rows, err := r.Auth.ListRetainedReleases(ctx, country)
	if err != nil {
		r.mu.Unlock()
		return "", "", fmt.Errorf("list retained releases: %w", err)
	}
	live := make(map[string]bool)
	if active != "" {
		if !validDBName(country, active) {
			r.mu.Unlock()
			return "", "", errors.New("invalid active database name")
		}
		live[active] = true
	}
	now := r.Clock()
	for _, row := range rows {
		if row.DroppedAt != nil || !row.RetainUntil.After(now) {
			continue
		}
		if !validDBName(country, row.DBName) {
			r.mu.Unlock()
			return "", "", errors.New("invalid retained database name")
		}
		live[row.DBName] = true
	}
	if r.live == nil {
		r.live = make(map[string]map[string]bool)
	}
	r.live[country] = live
	var retired []*cacheEntry
	for name, entry := range r.entries {
		if entry.country == country && !live[name] {
			delete(r.entries, name)
			if !entry.opening {
				retired = append(retired, entry)
			}
		}
	}
	name := active
	if rel != "" {
		name = "corpus_" + country + "_" + rel
	}
	allowed := live[name]
	r.mu.Unlock()
	for _, entry := range retired {
		entry.close()
	}
	if !allowed {
		return "", "", ErrExpired
	}
	return name, strings.TrimPrefix(name, "corpus_"+country+"_"), nil
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
