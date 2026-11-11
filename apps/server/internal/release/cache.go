package release

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"danny.vn/trachuan/internal/retrieval"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type cacheEntry struct {
	opening   bool
	country   string
	read      *retrieval.Engine
	search    *retrieval.Engine
	pool      *pgxpool.Pool
	closePool func()
	closeOnce sync.Once
}

func (entry *cacheEntry) close() {
	entry.closeOnce.Do(func() {
		if entry.closePool != nil {
			entry.closePool()
		}
	})
}

func (r *Resolver) load(ctx context.Context, country, name string, search bool) (any, error) {
	r.mu.Lock()
	if r.closed || !r.live[country][name] {
		r.mu.Unlock()
		return nil, ErrExpired
	}
	if r.entries == nil {
		r.entries = make(map[string]*cacheEntry)
	}
	entry := r.entries[name]
	if entry == nil {
		entry = &cacheEntry{country: country}
		r.entries[name] = entry
		if r.CloseDatabase != nil {
			entry.closePool = func() { r.CloseDatabase(name) }
		}
	}
	cached := entry.search
	if cached == nil && !search {
		cached = entry.read
	}
	entry.opening = true
	r.mu.Unlock()
	// The per-name single flight serializes opens, including a read-to-search
	// upgrade. A pool belongs to this entry until eviction or a failed first open.
	engine := cached
	var err error
	if cached != nil {
		if entry.pool != nil {
			err = entry.pool.Ping(ctx)
		}
	} else {
		engine, err = r.open(ctx, name, entry, search)
	}
	incompatible := false
	if search && err != nil && compatibilityFailure(err) {
		r.mu.Lock()
		read := entry.read
		r.mu.Unlock()
		if read == nil {
			read, err = r.open(ctx, name, entry, false)
		} else {
			err = nil
		}
		if err == nil {
			engine = read
			incompatible = true
		}
	}
	if err == nil && engine == nil {
		err = errors.New("opener returned no engine")
	}
	r.mu.Lock()
	entry.opening = false
	if r.closed || r.entries[name] != entry || !r.live[country][name] {
		r.mu.Unlock()
		entry.close()
		return nil, ErrExpired
	}
	if err != nil {
		empty := (entry.read == nil && entry.search == nil) || databaseGone(err)
		if empty {
			delete(r.entries, name)
		}
		r.mu.Unlock()
		if empty {
			entry.close()
		}
		if databaseGone(err) {
			return nil, ErrExpired
		}
		return nil, fmt.Errorf("open corpus engine: %w", err)
	}
	if search && !incompatible {
		entry.search = engine
	} else {
		entry.read = engine
	}
	r.mu.Unlock()
	if incompatible {
		return nil, ErrIncompatible
	}
	return nil, nil
}

func (r *Resolver) open(ctx context.Context, name string, entry *cacheEntry, search bool) (*retrieval.Engine, error) {
	if r.Open != nil || r.OpenRead != nil {
		if r.Open == nil || r.OpenRead == nil {
			return nil, errors.New("both corpus openers are required")
		}
		if search {
			return r.Open(name)
		}
		return r.OpenRead(name)
	}
	if entry.pool == nil {
		cfg, err := poolConfig(r.URLTemplate, name)
		if err != nil {
			return nil, err
		}
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("create corpus pool: %w", err)
		}
		r.mu.Lock()
		if r.closed || r.entries[name] != entry {
			r.mu.Unlock()
			pool.Close()
			return nil, ErrExpired
		}
		entry.pool = pool
		entry.closePool = pool.Close
		r.mu.Unlock()
	}
	options := r.Options
	if options.Now == nil {
		options.Now = r.Clock
	}
	if options.Profile == "" {
		options.Profile = "serving"
	}
	// Preserve a typed missing-database error before Open's legacy error paths.
	if err := entry.pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("connect corpus: %w", err)
	}
	var engine *retrieval.Engine
	var err error
	if search {
		engine, err = retrieval.Open(ctx, entry.pool, r.Embedder, options)
	} else {
		engine, err = retrieval.OpenForRead(ctx, entry.pool, options)
	}
	if err != nil {
		if search && err.Error() == "read release failed" {
			if pingErr := entry.pool.Ping(ctx); databaseGone(pingErr) {
				return nil, pingErr
			}
		}
		return nil, err
	}
	metadata := engine.Release()
	if metadata.Country != entry.country || "corpus_"+metadata.Country+"_"+metadata.Rel != name {
		return nil, errors.New("corpus release identity differs from database name")
	}
	return engine, nil
}

func poolConfig(template, name string) (*pgxpool.Config, error) {
	if strings.Count(template, "{database}") != 1 {
		return nil, errors.New("corpus URL requires one database placeholder")
	}
	cfg, err := pgxpool.ParseConfig(strings.Replace(template, "{database}", name, 1))
	if err != nil {
		return nil, errors.New("invalid corpus URL template")
	}
	if cfg.ConnConfig.Database != name {
		return nil, errors.New("corpus URL placeholder must select the database")
	}
	cfg.MaxConns = 4
	cfg.MinConns = 0
	cfg.MinIdleConns = 0
	return cfg, nil
}

func compatibilityFailure(err error) bool {
	// Open predates typed recipe errors. Only these validation failures can be
	// reclassified, and only after OpenForRead independently validates the corpus.
	if errors.Is(err, ErrIncompatible) {
		return true
	}
	for cause := err; cause != nil; cause = errors.Unwrap(cause) {
		if cause.Error() == "invalid dense recipe" || cause.Error() == "invalid release metadata" {
			return true
		}
	}
	return false
}

func databaseGone(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "3D000" || pgErr.Code == "57P01" || pgErr.Code == "57P04")
}
