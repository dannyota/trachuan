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

func shapeWithPolicy(arms Arms, recipe Recipe, limit int, mode ArmMode, cap, tail int, article ...*ArticleCapOverride) (Response, error) {
	if limit < 0 || limit > MaxLimit {
		return Response{}, errors.New("invalid ranking options")
	}
	plan, err := planWithPolicy(arms, recipe, limit, mode, cap, tail, article...)
	if err != nil {
		return Response{}, err
	}
	return plan.shape(arms, recipe)
}

type responsePlan struct {
	response            Response
	primary, nonCurrent []Group
}

func (p responsePlan) shape(arms Arms, recipe Recipe) (Response, error) {
	out := p.response
	out.Hits, out.NonCurrent = []Hit{}, []Hit{}
	out.Nodes = map[string]NodeRef{}
	for i, groups := range [][]Group{p.primary, p.nonCurrent} {
		for _, g := range groups {
			h, err := shapeHit(g, arms, recipe)
			if err != nil {
				return Response{}, err
			}
			for _, code := range h.Nodes {
				if ref, ok := arms.Nodes[code]; ok {
					out.Nodes[code] = ref
				}
			}
			if i == 0 {
				out.Hits = append(out.Hits, h)
			} else {
				out.NonCurrent = append(out.NonCurrent, h)
			}
		}
	}
	return out, nil
}

func planWithPolicy(arms Arms, recipe Recipe, limit int, mode ArmMode, cap, tail int, article ...*ArticleCapOverride) (responsePlan, error) {
	if tail < 0 || tail > 5 {
		return responsePlan{}, errors.New("invalid non-current tail")
	}
	if limit == 0 {
		limit = 8
	}
	tail = min(tail, max(0, limit/2))
	if arms.IncludeNonCurrent {
		tail = limit / 2
	}
	if (tail == 0 && !arms.IncludeNonCurrent) || !arms.Validation.Accepted {
		return planWithCap(arms, recipe, limit, mode, cap, article...)
	}
	// Validate and rank primary before ranking the complement.
	primary, err := rankWithCap(arms, recipe, limit, mode, cap, article...)
	if err != nil {
		return responsePlan{}, err
	}
	other, err := rankGroup(arms.NonCurrent, arms.Validation.Routed, recipe, limit, mode, cap, article...)
	if err != nil {
		return responsePlan{}, err
	}
	if arms.IncludeNonCurrent {
		tail = max(tail, limit-len(primary.Groups))
	}
	count := min(tail, len(other.Groups))
	primaryLimit := limit - count
	out, err := planWithCap(arms, recipe, max(1, primaryLimit), mode, cap, article...)
	if err != nil {
		return responsePlan{}, err
	}
	if len(out.primary) < primaryLimit {
		seen := map[int64]bool{}
		for _, group := range out.primary {
			seen[group.Best.GroupID] = true
		}
		for _, group := range primary.Groups {
			if len(out.primary) == primaryLimit {
				break
			}
			if !seen[group.Best.GroupID] {
				out.primary = append(out.primary, group)
			}
		}
		out.response.Diagnostics.EligibleCount = len(out.primary)
	}
	if arms.IncludeNonCurrent {
		count = min(len(other.Groups), limit-len(out.primary))
	}
	out.nonCurrent = append(out.nonCurrent, other.Groups[:count]...)
	return out, nil
}

func planWithCap(arms Arms, recipe Recipe, limit int, mode ArmMode, cap int, article ...*ArticleCapOverride) (responsePlan, error) {
	out := responsePlan{response: Response{Release: arms.Release, Hits: []Hit{}, NonCurrent: []Hit{}, Gaps: []Gap{}, Diagnostics: Diagnostics{DenseCount: len(arms.Primary.Dense), LexicalCount: len(arms.Primary.Lexical), RawANNCount: len(arms.Primary.RawANN), Fallback: arms.Primary.Fallback, QueryRecipeHash: arms.QueryRecipeHash, TimingsMS: map[string]int64{}}}}
	if !arms.Validation.Accepted {
		out.response.Abstain = true
		out.response.Reason = arms.Validation.Reason
		out.response.Message = arms.Validation.Message
		return out, nil
	}
	if recipe.Abstention.Threshold == nil || math.IsNaN(*recipe.Abstention.Threshold) || math.IsInf(*recipe.Abstention.Threshold, 0) {
		return responsePlan{}, errors.New("uncalibrated retrieval recipe")
	}
	ranking, err := rankWithCap(arms, recipe, limit, mode, cap, article...)
	if err != nil {
		return responsePlan{}, err
	}
	out.response.Diagnostics.EligibleCount = len(ranking.Groups)
	if arms.AsOf != nil {
		out.response.Gaps = append(out.response.Gaps, Gap{Kind: "historical_document_dates", Message: "Historical filtering uses document dates, not provision history or past suspension status."})
	}
	out.primary = ranking.Groups
	if ranking.FirstBestCosine == nil {
		out.response.Abstain = true
		out.response.Reason = "no_primary_hits"
	} else if *ranking.FirstBestCosine < *recipe.Abstention.Threshold {
		out.response.Abstain = true
		out.response.Reason = "low_confidence"
	}
	return out, nil
}

func shapeHit(g Group, arms Arms, recipe Recipe) (Hit, error) {
	c := g.Best
	if g.article != nil {
		c = *g.article
	}
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
	nodes := append([]string{}, arms.EffectiveNodes[c.GroupID]...)
	h := Hit{Nodes: nodes, Locator: locator, Identity: g.Identity, Path: path, BestMember: bestMember, BestTextStart: bestStart, Number: c.Document.Number, Class: c.Document.Class, Source: c.Document.Source, Title: c.Document.Title, Citation: citation(c.Ancestors), Text: text, Fragments: fragments, Complete: complete, SourceURL: c.Document.SourceURL, Score: g.Score, BestCosine: g.BestCosine, Stamp: validity, Relations: rels, Warnings: c.Warnings, WarningsOmitted: c.WarningsOmitted, Gaps: gaps}
	if !complete {
		v := 1
		h.NextPart = &v
	}
	return h, nil
}
