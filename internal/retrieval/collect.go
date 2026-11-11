package retrieval

import (
	"context"
	"errors"
	"strconv"
	"time"

	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/jackc/pgx/v5"
)

// Collect caches both candidate arms without calling the embedder.
func (e *Engine) Collect(ctx context.Context, p Prepared, depths Depths) (Arms, error) {
	if !p.Validation.Accepted {
		return Arms{Release: e.release.Rel, RecipeHash: e.release.RetrievalRecipe.Hash(), QueryRecipeHash: e.queryRecipe.Hash(), Validation: p.Validation}, nil
	}
	if e.db == nil || p.Vector == nil || len(p.Vector.Values) != e.queryRecipe.Dimensions || p.Vector.RecipeHash != e.queryRecipe.Hash() || depths.Dense < 1 || depths.Dense > 200 || depths.Lexical < 1 || depths.Lexical > 100 {
		return Arms{}, errors.New("invalid prepared request or depths")
	}
	sparse, err := lexical.Query(p.Request.Query, e.release.LexicalRecipe)
	if err != nil {
		return Arms{}, errors.New("encode lexical query failed")
	}
	date := e.options.Now().In(time.FixedZone("Vietnam", 7*60*60)).Format("2006-01-02")
	historical := p.Request.AsOf != nil
	if historical {
		d := p.Request.AsOf
		date = time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	}
	ctx, cancel := context.WithTimeout(ctx, e.deadline())
	defer cancel()
	tx, err := e.db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return Arms{}, databaseFailure("begin retrieval failed", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err = setLocal(ctx, tx, "statement_timeout", strconv.FormatInt(e.deadline().Milliseconds(), 10)); err != nil {
		return Arms{}, err
	}
	result := Arms{Release: e.release.Rel, RecipeHash: e.release.RetrievalRecipe.Hash(), QueryRecipeHash: e.queryRecipe.Hash(), Validation: p.Validation, Depths: depths}
	parsedDate, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return Arms{}, errors.New("invalid evaluation date")
	}
	result.Date = Date{Year: parsedDate.Year(), Month: parsedDate.Month(), Day: parsedDate.Day()}
	result.AsOf = p.Request.AsOf
	result.IncludeNonCurrent = p.Request.InForceOnly != nil && !*p.Request.InForceOnly
	groups := []*ArmGroup{&result.Primary}
	flags := []bool{true}
	if p.Request.InForceOnly != nil && !*p.Request.InForceOnly {
		groups = append(groups, &result.NonCurrent)
		flags = append(flags, false)
	}
	refs := append([]string{}, p.Validation.References...)
	vec := denseVector(p.Vector.Values)
	sv := sparseVector(sparse, e.release.LexicalRecipe.Dimensions)
	exact := p.Request.Mode == "exact"
	var pool int
	if exact {
		err = exactSettings(ctx, tx)
	} else {
		pool, err = annSettings(ctx, tx, depths.Dense, e.release.RetrievalRecipe.ANN)
	}
	if err != nil {
		return Arms{}, err
	}
	for i, g := range groups {
		args := []any{vec, date, historical, flags[i], depths.Dense, refs}
		if exact {
			g.Dense, err = denseQuery(ctx, tx, exactSQL, args...)
		} else {
			g.RawANN, err = denseQuery(ctx, tx, annSQL, append(args, pool)...)
			g.Dense = g.RawANN
		}
		if err != nil {
			return Arms{}, err
		}
		g.Lexical, err = lexicalQuery(ctx, tx, sv, date, historical, flags[i], depths.Lexical, refs)
		if err != nil {
			return Arms{}, err
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
				return Arms{}, err
			}
			for i, g := range groups {
				if len(g.RawANN) < depths.Dense {
					g.Fallback = true
					g.Dense, err = denseQuery(ctx, tx, exactSQL, vec, date, historical, flags[i], depths.Dense, refs)
					if err != nil {
						return Arms{}, err
					}
				}
			}
		}
	}
	if err = indexSettings(ctx, tx); err != nil {
		return Arms{}, err
	}
	for _, g := range groups {
		if err = hydrate(ctx, tx, g, vec); err != nil {
			return Arms{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Arms{}, databaseFailure("commit retrieval failed", err)
	}
	return result, nil
}
