package retrieval

import (
	"context"
	"errors"
	"fmt"
)

// Search validates, collects and shapes a request using the release recipe.
func (e *Engine) Search(ctx context.Context, req Request) (Response, error) {
	recipe := e.release.RetrievalRecipe
	if recipe.Abstention.Threshold == nil {
		return Response{}, errors.New("uncalibrated retrieval recipe")
	}
	req, c, err := e.searchRequest(req)
	if err != nil {
		return Response{}, err
	}
	prepared, err := e.Prepare(ctx, req)
	if err != nil {
		return Response{}, fmt.Errorf("prepare search: %w", err)
	}
	arms, err := e.Collect(ctx, prepared, Depths{Dense: recipe.DenseDepth, Lexical: recipe.LexicalDepth})
	if err != nil {
		return Response{}, fmt.Errorf("collect search: %w", err)
	}
	walk, err := buildSearchWalk(arms, recipe, c.FirstLimit, req.Arms)
	if err != nil {
		return Response{}, fmt.Errorf("plan search: %w", err)
	}
	var docs map[string]DocumentEntry
	if len(req.DocTypes)+len(req.Issuers) > 0 {
		var keys []string
		for _, gs := range [][]Group{walk.primary, walk.nonCurrent} {
			for _, g := range gs {
				keys = append(keys, g.Identity.DocumentKey)
			}
		}
		docs, err = e.Documents(ctx, keys)
		if err != nil {
			return Response{}, fmt.Errorf("load filter documents: %w", err)
		}
	}
	walk.filter(req, arms.EffectiveNodes, docs)
	response, err := walk.page(arms, recipe, req, c)
	if err != nil {
		return Response{}, fmt.Errorf("shape search: %w", err)
	}
	if err := e.LoadDecisions(ctx, response.Hits, response.NonCurrent); err != nil {
		return Response{}, fmt.Errorf("load validity decisions: %w", err)
	}
	return response, nil
}
