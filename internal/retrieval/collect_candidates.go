package retrieval

import (
	"context"
	"time"

	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/jackc/pgx/v5"
)

func (e *Engine) collectCandidates(ctx context.Context, tx pgx.Tx, p Prepared, vec string, sparse lexical.Vector, depths Depths, date time.Time, notYet bool, source *LexicalArms) ([2]ArmGroup, func(), error) {
	var result [2]ArmGroup
	day := date.Format(time.DateOnly)
	historical := p.Request.AsOf != nil
	groups := []*ArmGroup{&result[0], &result[1]}
	flags := []bool{true, false}
	refs := append([]string{}, p.Validation.References...)
	sv := sparseVector(sparse, e.release.LexicalRecipe.Dimensions)
	postings := e.postingsFor(sparse)
	exact := p.Request.Mode == "exact"
	var pool int
	var err error
	if exact {
		err = exactSettings(ctx, tx)
	} else {
		pool, err = annSettings(ctx, tx, depths.Dense, e.release.RetrievalRecipe.ANN)
	}
	if err != nil {
		return result, nil, err
	}
	if !exact {
		if err = annQuery(ctx, tx, groups, depths.Dense, validitySQL(annSQL, notYet), vec, day, historical, refs, pool); err != nil {
			return result, nil, err
		}
	}
	for i, g := range groups {
		args := []any{vec, day, historical, flags[i], depths.Dense, refs}
		if exact {
			g.Dense, err = denseQuery(ctx, tx, validitySQL(exactSQL, notYet), args...)
		} else {
			g.Dense = g.RawANN
		}
		if err != nil {
			return result, nil, err
		}
		if source != nil {
			candidates := source.Primary
			if i == 1 {
				candidates = source.NonCurrent
			}
			g.Lexical = lexicalPrefix(candidates, depths.Lexical)
		} else if postings != nil {
			continue
		} else if notYet {
			g.Lexical, err = lexicalQueryWithValidity(ctx, tx, true, sv, day, historical, flags[i], depths.Lexical, refs)
		} else {
			g.Lexical, err = lexicalQuery(ctx, tx, sv, day, historical, flags[i], depths.Lexical, refs)
		}
		if err != nil {
			return result, nil, err
		}
	}
	if !exact {
		need := false
		for _, g := range groups {
			if len(g.RawANN) < depths.Dense {
				need = true
			}
		}
		if need {
			if err = exactSettings(ctx, tx); err != nil {
				return result, nil, err
			}
			for i, g := range groups {
				if len(g.RawANN) < depths.Dense {
					g.Fallback = true
					g.Dense, err = denseQuery(ctx, tx, validitySQL(exactSQL, notYet), vec, day, historical, flags[i], depths.Dense, refs)
					if err != nil {
						return result, nil, err
					}
				}
			}
		}
	}
	if err = indexSettings(ctx, tx); err != nil {
		return result, nil, err
	}
	if source == nil && postings != nil {
		lexical, scoreErr := postings.lexical(ctx, tx, sparse, sv, date, historical, notYet, refs, depths.Lexical)
		if scoreErr != nil {
			return result, nil, scoreErr
		}
		for i, g := range groups {
			g.Lexical = lexical[i]
		}
	}

	return result, func() {
		if source != nil {
			e.sparse.reused.Add(1)
		} else if postings != nil {
			e.sparse.postings.Add(1)
		} else {
			e.sparse.sql.Add(1)
		}
	}, nil
}
