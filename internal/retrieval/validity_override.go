package retrieval

import (
	"context"
	"fmt"
	"strings"
)

// CollectWithNotYetEligible collects research candidates under the evaluation profile.
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
	const trial = `((($3::boolean OR (d.is_current AND NOT d.validity_conflict)) AND (d.eff_from IS NULL OR d.eff_from <= $2::date) AND (d.eff_to IS NULL OR $2::date < d.eff_to)) OR (NOT $3::boolean AND d.status_class = 'not_yet')) = $4::boolean`
	return strings.Replace(query, eligibility, trial, 1)
}

// ShapeWithNonCurrentTail reserves response slots for non-current evidence.
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
