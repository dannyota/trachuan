package retrieval

import (
	"context"
	"errors"
	"strconv"
	"time"

	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/jackc/pgx/v5"
)

func (e *Engine) Collect(ctx context.Context, p Prepared, depths Depths) (Arms, error) {
	return e.collect(ctx, p, depths, false)
}

func (e *Engine) collect(ctx context.Context, p Prepared, depths Depths, notYet bool) (Arms, error) {
	return e.collectWithLexical(ctx, p, depths, notYet, nil)
}

func (e *Engine) collectWithLexical(ctx context.Context, p Prepared, depths Depths, notYet bool, source *LexicalArms) (Arms, error) {
	return e.collectWithCandidates(ctx, p, depths, notYet, source, nil)
}

func (e *Engine) collectWithCandidates(ctx context.Context, p Prepared, depths Depths, notYet bool, source *LexicalArms, cache CandidateCache) (Arms, error) {
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
	date := e.searchDate(p.Request)
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
	groups := []*ArmGroup{&result.Primary, &result.NonCurrent}
	vec := denseVector(p.Vector.Values)
	candidates, complete, err := e.candidates(ctx, tx, p, vec, sparse, depths, parsedDate, notYet, source, cache)
	if err != nil {
		return Arms{}, err
	}
	result.Primary, result.NonCurrent = candidates[0], candidates[1]
	for _, g := range groups {
		if err = hydrate(ctx, tx, g, vec); err != nil {
			return Arms{}, err
		}
	}
	var groupIDs []int64
	seenGroups := map[int64]bool{}
	for _, g := range groups {
		for _, c := range g.Members {
			if !seenGroups[c.GroupID] {
				seenGroups[c.GroupID] = true
				groupIDs = append(groupIDs, c.GroupID)
			}
		}
	}
	if e.release.RetrievalRecipe.SiblingMerge == "article-clauses/1" {
		articleIDs, hydrateErr := hydrateSiblingArticles(ctx, tx, &result)
		if hydrateErr != nil {
			return Arms{}, hydrateErr
		}
		for _, id := range articleIDs {
			if !seenGroups[id] {
				groupIDs = append(groupIDs, id)
			}
		}
	}
	result.EffectiveNodes, result.Nodes, err = e.loadEffectiveNodes(ctx, tx, groupIDs)
	if err != nil {
		return Arms{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Arms{}, databaseFailure("commit retrieval failed", err)
	}
	if complete != nil {
		complete()
	}
	return result, nil
}

func (e *Engine) searchDate(req Request) string {
	if req.AsOf != nil {
		return req.AsOf.String()
	}
	instant := req.Instant
	if instant.IsZero() {
		instant = e.options.Now()
	}
	return instant.In(time.FixedZone("Asia/Ho_Chi_Minh", 7*60*60)).Format(time.DateOnly)
}
