package retrieval

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"time"

	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/jackc/pgx/v5"
)

type CandidateCache interface {
	Get(release string, key [32]byte) ([]byte, bool)
	Put(release string, key [32]byte, value []byte)
}

type candidatePublication struct {
	CandidateCache
	release string
	key     [32]byte
	value   []byte
}

func (c *candidatePublication) Put(release string, key [32]byte, value []byte) {
	c.release, c.key, c.value = release, key, value
}

func (c *candidatePublication) publish(ctx context.Context) {
	if c.value != nil && ctx.Err() == nil {
		c.CandidateCache.Put(c.release, c.key, c.value)
	}
}

func (e *Engine) candidateCache(ctx context.Context) CandidateCache {
	if e.options.Profile != "serving" {
		return nil
	}
	if caches, ok := ctx.Value(cacheContextKey{}).(requestCaches); ok {
		return caches.candidates
	}
	return e.options.Candidates
}

func (e *Engine) candidateKey(req Request, depths Depths, date string) [32]byte {
	mode := req.Mode
	if mode == "" {
		mode = "ann"
	}
	raw, _ := json.Marshal(struct {
		QueryRecipe, RetrievalRecipe, LexicalRecipe string
		Query, CollectionDate                       string
		AsOf                                        *Date
		Mode                                        Mode
		DenseDepth, LexicalDepth                    int
	}{e.queryRecipe.Hash(), e.release.RetrievalRecipe.Hash(), e.release.LexicalRecipe.Hash(), req.Query, date, req.AsOf, mode, depths.Dense, depths.Lexical})
	return sha256.Sum256(raw)
}

func (e *Engine) candidates(ctx context.Context, tx pgx.Tx, p Prepared, vec string, sparse lexical.Vector, depths Depths, date time.Time, notYet bool, source *LexicalArms, cache CandidateCache) ([2]ArmGroup, func(), error) {
	if err := ctx.Err(); err != nil {
		return [2]ArmGroup{}, nil, err
	}
	if e.options.Profile != "serving" || notYet || source != nil || !p.Validation.Accepted {
		cache = nil
	}
	var key [32]byte
	if cache != nil {
		key = e.candidateKey(p.Request, depths, date.Format(time.DateOnly))
		if err := ctx.Err(); err != nil {
			return [2]ArmGroup{}, nil, err
		}
		if raw, ok := cache.Get(e.release.Rel, key); ok {
			if groups, valid := decodeCandidates(raw); valid {
				if err := indexSettings(ctx, tx); err != nil {
					return [2]ArmGroup{}, nil, err
				}
				return groups, nil, nil
			}
		}
	}
	groups, complete, err := e.collectCandidates(ctx, tx, p, vec, sparse, depths, date, notYet, source)
	if err != nil || cache == nil {
		return groups, complete, err
	}
	raw := encodeCandidates(groups)
	return groups, func() {
		complete()
		if ctx.Err() == nil {
			cache.Put(e.release.Rel, key, raw)
		}
	}, nil
}
