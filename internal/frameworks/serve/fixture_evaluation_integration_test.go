//go:build integration

package serve

import (
	"context"

	"danny.vn/trachuan/internal/frameworks/build"
)

func (d fixtureDatabase) Evaluate(_ context.Context, _ build.EvaluationRequest, publish func(build.EvaluationResult) error) error {
	return publish(build.EvaluationResult{RetrievalRecipeStatus: "placeholder"})
}
