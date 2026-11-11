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
	prepared, err := e.Prepare(ctx, req)
	if err != nil {
		return Response{}, fmt.Errorf("prepare search: %w", err)
	}
	arms, err := e.Collect(ctx, prepared, Depths{Dense: recipe.DenseDepth, Lexical: recipe.LexicalDepth})
	if err != nil {
		return Response{}, fmt.Errorf("collect search: %w", err)
	}
	response, err := Shape(arms, recipe, req.Limit, req.Arms)
	if err != nil {
		return Response{}, fmt.Errorf("shape search: %w", err)
	}
	if err := e.LoadDecisions(ctx, response.Hits, response.NonCurrent); err != nil {
		return Response{}, fmt.Errorf("load validity decisions: %w", err)
	}
	return response, nil
}
