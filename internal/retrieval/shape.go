package retrieval

import (
	"errors"
	"math"
	"strings"
)

// Shape constructs a bounded evidence response from cached candidates.
func Shape(arms Arms, recipe Recipe, limit int, mode ArmMode) (Response, error) {
	return shapeWithPolicy(arms, recipe, limit, mode, recipe.DocumentCap, recipe.NonCurrentTail)
}

func shapeWithPolicy(arms Arms, recipe Recipe, limit int, mode ArmMode, cap, tail int) (Response, error) {
	if tail < 0 || tail > 5 {
		return Response{}, errors.New("invalid non-current tail")
	}
	if limit == 0 {
		limit = 8
	}
	if arms.IncludeNonCurrent || tail == 0 || !arms.Validation.Accepted {
		return shapeWithCap(arms, recipe, limit, mode, cap)
	}
	// Validate and rank primary before ranking the complement.
	if _, err := rankWithCap(arms, recipe, limit, mode, cap); err != nil {
		return Response{}, err
	}
	other, err := rankGroup(arms.NonCurrent, arms.Validation.Routed, recipe, limit, mode, cap)
	if err != nil {
		return Response{}, err
	}
	count := min(tail, len(other.Groups), limit)
	primaryLimit := limit - count
	out, err := shapeWithCap(arms, recipe, max(1, primaryLimit), mode, cap)
	if err != nil {
		return Response{}, err
	}
	if primaryLimit == 0 {
		out.Hits = out.Hits[:0]
	}
	for _, group := range other.Groups[:count] {
		hit, shapeErr := shapeHit(group, arms, recipe)
		if shapeErr != nil {
			return Response{}, shapeErr
		}
		out.NonCurrent = append(out.NonCurrent, hit)
	}
	return out, nil
}

func shapeWithCap(arms Arms, recipe Recipe, limit int, mode ArmMode, cap int) (Response, error) {
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
	ranking, err := rankWithCap(arms, recipe, limit, mode, cap)
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
		other, rankErr := rankGroup(arms.NonCurrent, arms.Validation.Routed, recipe, limit, mode, cap)
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
	text, fragments, complete, evidenceGaps, bestMember, bestStart, err := evidence(g, recipe.EvidenceRuneCap)
	if err != nil {
		return Hit{}, err
	}
	gaps = append(gaps, evidenceGaps...)
	rels, relationGaps := shapeRelations(c.Relations, recipe, g.Identity)
	gaps = append(gaps, relationGaps...)
	path := make([]PathNode, 0, len(c.Ancestors))
	for _, p := range c.Ancestors {
		path = append(path, PathNode{Identity: p.Identity, Kind: p.Kind, Label: strings.TrimSpace(p.Label), Heading: strings.TrimSpace(p.Heading)})
	}
	var locator *string
	for _, p := range c.Descendants {
		if p.ID == c.GroupID {
			locator = p.Locator
			break
		}
	}
	h := Hit{Locator: locator, Identity: g.Identity, Path: path, BestMember: bestMember, BestTextStart: bestStart, Number: c.Document.Number, Class: c.Document.Class, Source: c.Document.Source, Title: c.Document.Title, Citation: citation(c.Ancestors), Text: text, Fragments: fragments, Complete: complete, SourceURL: c.Document.SourceURL, Score: g.Score, BestCosine: g.BestCosine, Stamp: validity, Relations: rels, Gaps: gaps}
	if !complete {
		v := 1
		h.NextPart = &v
	}
	return h, nil
}
