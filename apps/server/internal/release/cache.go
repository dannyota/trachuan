package release

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/puddle/v2"
)

type cacheEntry struct {
	opening   bool
	retired   bool
	leases    int
	country   string
	engine    *retrieval.Engine
	pool      *pgxpool.Pool
	closePool func()
	closeOnce sync.Once
}

func (entry *cacheEntry) close() {
	entry.closeOnce.Do(func() {
		if entry.engine != nil {
			entry.engine.Close()
		}
		if entry.closePool != nil {
			entry.closePool()
		}
	})
}

func (r *Resolver) load(ctx context.Context, country, name string) (*cacheEntry, error) {
	r.mu.Lock()
	if r.closed || r.active[country] != name {
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
	}
	engine := entry.engine
	hadEngine := engine != nil
	entry.opening = true
	r.mu.Unlock()
	var err error
	if engine != nil {
		if entry.pool != nil {
			err = entry.pool.Ping(ctx)
		}
	} else {
		engine, err = r.open(ctx, name, entry)
	}
	if err == nil {
		err = ctx.Err()
	}
	if err == nil && engine == nil {
		err = errors.New("opener returned no engine")
	}
	r.mu.Lock()
	entry.opening = false
	if engine != nil {
		entry.engine = engine
	}
	if r.closed || entry.retired || r.entries[name] != entry || r.active[country] != name {
		entry.retired = true
		if entry.engine != nil {
			entry.engine.Retire()
		}
		closePool := entry.leases == 0
		r.mu.Unlock()
		if closePool {
			entry.close()
		}
		return nil, ErrExpired
	}
	if err != nil {
		empty := !hadEngine || databaseGone(err)
		if empty {
			delete(r.entries, name)
			entry.retired = true
			if entry.engine != nil {
				entry.engine.Retire()
			}
		}
		closePool := entry.leases == 0
		r.mu.Unlock()
		if empty && closePool {
			entry.close()
		}
		if databaseGone(err) {
			return nil, ErrExpired
		}
		return nil, fmt.Errorf("open corpus engine: %w", err)
	}
	entry.engine = engine
	engine.StartSparsePostings(func(outcome retrieval.PostingsOutcome) {
		if r.OnPostings != nil {
			r.OnPostings(engine.Release().Rel, outcome)
		}
	})
	r.mu.Unlock()
	return entry, nil
}

func (r *Resolver) open(ctx context.Context, name string, entry *cacheEntry) (*retrieval.Engine, error) {
	if r.Open != nil {
		engine, closePool, err := r.Open(ctx, name)
		r.mu.Lock()
		entry.closePool = closePool
		r.mu.Unlock()
		return engine, err
	}
	if r.EmbedderFor == nil {
		return nil, errors.New("missing release embedder factory")
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
	if err := entry.pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("connect corpus: %w", err)
	}
	metadata, err := retrieval.ReadRelease(ctx, entry.pool)
	if err != nil {
		return nil, err
	}
	if !recipeAllowed(metadata.DenseRecipe.QueryHash, r.AllowTestRecipe) {
		if r.OnTestRecipeRefused != nil {
			r.OnTestRecipeRefused()
		}
		return nil, errors.New("test recipe refused")
	}
	embedder := r.EmbedderFor(metadata.DenseRecipe.QueryHash, metadata.DenseRecipe.Query)
	engine, err := retrieval.Open(ctx, entry.pool, embedder, options)
	if err != nil {
		return nil, err
	}
	metadata = engine.Release()
	if metadata.Country != entry.country || "corpus_"+metadata.Country+"_"+metadata.Rel != name {
		return engine, errors.New("corpus release identity differs from database name")
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

func databaseGone(err error) bool {
	if errors.Is(err, retrieval.ErrReleaseGone) || errors.Is(err, puddle.ErrClosedPool) {
		return true
	}
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "3D000" || pgErr.Code == "57P01" || pgErr.Code == "57P04")
}

func recipeAllowed(hash string, loopback bool) bool {
	return hash != embedding.TestRecipeHash || loopback
}
