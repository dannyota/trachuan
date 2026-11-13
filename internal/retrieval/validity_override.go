package retrieval

import (
	"context"
	"fmt"
	"strings"
)

func (e *Engine) CollectWithNotYetEligible(ctx context.Context, p Prepared, depths Depths) (Arms, error) {
	if e.options.Profile != "evaluation" || p.Request.AsOf != nil {
		return Arms{}, fmt.Errorf("not-yet eligibility requires evaluation without as_of")
	}
	return e.collect(ctx, p, depths, true)
}

func validitySQL(query string, notYet bool) string {
	if !notYet {
		return query
	}
	const trial = `(` + eligibilityExpression + ` OR (NOT $3::boolean AND d.status_class = 'not_yet'))`
	return strings.Replace(query, eligibilityExpression, trial, 1)
}

func ShapeWithNonCurrentTail(arms Arms, recipe Recipe, limit int, mode ArmMode, capOverride *CapOverride, tail int) (Response, error) {
	if limit == 0 {
		limit = 8
	}
	cap, err := effectiveCap(capOverride, recipe.DocumentCap, limit)
	if err != nil {
		return Response{}, err
	}
	arms.IncludeNonCurrent = false
	return shapeWithPolicy(arms, recipe, limit, mode, cap, tail)
}
