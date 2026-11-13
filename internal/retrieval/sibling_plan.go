package retrieval

import (
	"errors"
	"math"
)

// siblingPlan freezes both partitions at the requested limit before reserving slots.
func siblingPlan(arms Arms, recipe Recipe, limit int, mode ArmMode, cap, tail int, article ...*ArticleCapOverride) (responsePlan, error) {
	if limit == 0 {
		limit = 8
	}
	if tail < 0 || tail > 5 {
		return responsePlan{}, errors.New("invalid non-current tail")
	}
	if recipe.Abstention.Threshold == nil || math.IsNaN(*recipe.Abstention.Threshold) || math.IsInf(*recipe.Abstention.Threshold, 0) {
		return responsePlan{}, errors.New("uncalibrated retrieval recipe")
	}
	primary, err := rankWithCap(arms, recipe, limit, mode, cap, article...)
	if err != nil {
		return responsePlan{}, err
	}
	tail = min(tail, limit/2)
	if arms.IncludeNonCurrent {
		tail = limit / 2
	}
	other := Ranking{}
	if tail > 0 || arms.IncludeNonCurrent {
		other, err = rankGroup(arms.NonCurrent, arms.Validation.Routed, recipe, limit, mode, cap, article...)
		if err != nil {
			return responsePlan{}, err
		}
	}
	if arms.IncludeNonCurrent {
		tail = max(tail, limit-len(primary.Groups))
	}
	count := min(tail, len(other.Groups))
	primaryCount := min(len(primary.Groups), limit-count)
	if arms.IncludeNonCurrent {
		count = min(len(other.Groups), limit-primaryCount)
	}
	out := Response{Release: arms.Release, Hits: []Hit{}, NonCurrent: []Hit{}, Gaps: []Gap{}, Diagnostics: Diagnostics{DenseCount: len(arms.Primary.Dense), LexicalCount: len(arms.Primary.Lexical), RawANNCount: len(arms.Primary.RawANN), Fallback: arms.Primary.Fallback, QueryRecipeHash: arms.QueryRecipeHash, TimingsMS: map[string]int64{}, EligibleCount: primaryCount}}
	if arms.AsOf != nil {
		out.Gaps = append(out.Gaps, Gap{Kind: "historical_document_dates", Message: "Historical filtering uses document dates, not provision history or past suspension status."})
	}
	if primary.FirstBestCosine == nil {
		out.Abstain = true
		out.Reason = "no_primary_hits"
	} else if *primary.FirstBestCosine < *recipe.Abstention.Threshold {
		out.Abstain = true
		out.Reason = "low_confidence"
	}
	return responsePlan{response: out, primary: primary.Groups[:primaryCount], nonCurrent: other.Groups[:count]}, nil
}

// ShapeGroups returns the frozen primary groups before document and output caps.
func ShapeGroups(arms Arms, recipe Recipe, limit int, mode ArmMode) ([]Hit, error) {
	ranking, err := rankWithPolicy(arms, recipe, limit, mode, recipe.DocumentCap, rankPolicy{allGroups: true})
	if err != nil {
		return nil, err
	}
	out, err := (responsePlan{primary: ranking.Groups}).shape(arms, recipe)
	return out.Hits, err
}
