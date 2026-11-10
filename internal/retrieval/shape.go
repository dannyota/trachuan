package retrieval

import (
	"errors"
	"math"
)

// Shape constructs a bounded evidence response from cached candidates.
func Shape(arms Arms, recipe Recipe, limit int, mode ArmMode) (Response, error) {
	out := Response{Release: arms.Release, Hits: []Hit{}, NonCurrent: []Hit{}, Gaps: []Gap{}, Diagnostics: Diagnostics{DenseCount: len(arms.Primary.Dense), LexicalCount: len(arms.Primary.Lexical), RawANNCount: len(arms.Primary.RawANN), Fallback: arms.Primary.Fallback, QueryRecipeHash: arms.QueryRecipeHash, TimingsMS: map[string]int64{}}}
	if !arms.Validation.Accepted {
		out.Abstain = true
		out.Reason = arms.Validation.Reason
		out.Message = arms.Validation.Message
		return out, nil
	}
	if recipe.Abstention.Threshold == nil || math.IsNaN(*recipe.Abstention.Threshold) || math.IsInf(*recipe.Abstention.Threshold, 0) {
		return Response{}, errors.New("uncalibrated retrieval recipe")
	}
	ranking, err := Rank(arms, recipe, limit, mode)
	if err != nil {
		return Response{}, err
	}
	if limit == 0 {
		limit = 8
	}
	out.Diagnostics.EligibleCount = len(ranking.Groups)
	if arms.AsOf != nil {
		out.Gaps = append(out.Gaps, Gap{Kind: "historical_document_dates", Message: "Historical filtering uses document dates, not provision history or past suspension status."})
	}
	for _, g := range ranking.Groups {
		hit, shapeErr := shapeHit(g, arms, recipe)
		if shapeErr != nil {
			return Response{}, shapeErr
		}
		out.Hits = append(out.Hits, hit)
	}
	if arms.IncludeNonCurrent && len(out.Hits) < limit {
		other, rankErr := rankGroup(arms.NonCurrent, arms.Validation.Routed, recipe, limit, mode)
		if rankErr != nil {
			return Response{}, rankErr
		}
		for _, g := range other.Groups {
			if len(out.Hits)+len(out.NonCurrent) >= limit {
				break
			}
			hit, shapeErr := shapeHit(g, arms, recipe)
			if shapeErr != nil {
				return Response{}, shapeErr
			}
			out.NonCurrent = append(out.NonCurrent, hit)
		}
	}
	if ranking.FirstBestCosine == nil {
		out.Abstain = true
		out.Reason = "no_primary_hits"
	} else if *ranking.FirstBestCosine < *recipe.Abstention.Threshold {
		out.Abstain = true
		out.Reason = "low_confidence"
	}
	return out, nil
}

func shapeHit(g Group, arms Arms, recipe Recipe) (Hit, error) {
	c := g.Best
	validity, gaps := stamp(c.Document, arms)
	for i := range gaps {
		gaps[i].DocumentKey = g.Identity.DocumentKey
		gaps[i].Anchor = g.Identity.Anchor
	}
	text, fragments, complete, evidenceGaps, err := evidence(g, recipe.EvidenceRuneCap)
	if err != nil {
		return Hit{}, err
	}
	gaps = append(gaps, evidenceGaps...)
	rels, relationGaps := shapeRelations(c.Relations, recipe, g.Identity)
	gaps = append(gaps, relationGaps...)
	h := Hit{Identity: g.Identity, Number: c.Document.Number, Class: c.Document.Class, Source: c.Document.Source, Title: c.Document.Title, Citation: citation(c.Ancestors), Text: text, Fragments: fragments, Complete: complete, SourceURL: c.Document.SourceURL, Score: g.Score, BestCosine: g.BestCosine, Stamp: validity, Relations: rels, Gaps: gaps}
	if !complete {
		v := 1
		h.NextPart = &v
	}
	return h, nil
}
