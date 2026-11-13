package retrieval

import (
	"errors"
	"math"
)

// SiblingMerge holds article metadata for the evaluation-only sibling override.
// Articles contain no additional ranked chunks or merge members.
type SiblingMerge struct {
	Articles       map[int64]Candidate
	EffectiveNodes map[int64][]string
	Nodes          map[string]NodeRef
}

func groupArticle(g Group) (Provision, bool) {
	// Ancestry ends at the group, even when the best chunk belongs to a point.
	for i := len(g.Best.Ancestors) - 1; i >= 0; i-- {
		p := g.Best.Ancestors[i]
		if p.Kind == "dieu" {
			return p, true
		}
	}
	return Provision{}, false
}

func mergeSiblingGroups(groups []Group) []Group {
	type articleKey struct {
		identity Identity
		id       int64
	}
	keys := make([]articleKey, len(groups))
	clauses := map[articleKey]int{}
	veto := map[articleKey]bool{}
	for i, g := range groups {
		article, ok := groupArticle(g)
		if !ok {
			continue
		}
		key := articleKey{article.Identity, article.ID}
		keys[i] = key
		if g.Best.GroupID == article.ID {
			veto[key] = true
		}
		for _, p := range g.Best.Ancestors {
			if p.ID == g.Best.GroupID && p.Kind == "khoan" {
				clauses[key]++
			}
		}
	}
	out := make([]Group, 0, len(groups))
	indices := map[articleKey]int{}
	for i, g := range groups {
		key := keys[i]
		isClause := false
		for _, p := range g.Best.Ancestors {
			if p.ID == g.Best.GroupID && p.Kind == "khoan" {
				isClause = true
			}
		}
		if !isClause || clauses[key] < 2 || veto[key] {
			out = append(out, g)
			continue
		}
		if index, ok := indices[key]; ok {
			target := &out[index]
			target.siblings = append(target.siblings, g)
			target.Members = append(target.Members, g.Members...)
			target.Score = max(target.Score, g.Score)
			target.BestCosine = max(target.BestCosine, g.BestCosine)
		} else {
			indices[key] = len(out)
			merged := g
			merged.Identity = key.identity
			merged.Members = append([]Candidate(nil), g.Members...)
			merged.siblings = []Group{g}
			out = append(out, merged)
		}
	}
	return out
}

// RankWithSiblingMerge ranks with frozen sibling membership before document capping.
// It leaves recipes and the ranking used by builds unchanged.
func RankWithSiblingMerge(arms Arms, recipe Recipe, limit int, mode ArmMode, capOverride *CapOverride) (Ranking, error) {
	if limit == 0 {
		limit = 8
	}
	cap, err := effectiveCap(capOverride, recipe.DocumentCap, limit)
	if err != nil {
		return Ranking{}, err
	}
	return rankWithSiblingPolicy(arms, recipe, limit, mode, cap, true)
}

// Shape applies the research merge within the original requested shaping window.
// A non-nil tail selects the same override semantics as ShapeWithNonCurrentTail.
func (m *SiblingMerge) Shape(arms Arms, recipe Recipe, limit int, mode ArmMode, capOverride *CapOverride, tailOverride *int) (Response, error) {
	if limit == 0 {
		limit = 8
	}
	if !arms.Validation.Accepted {
		return ShapeWithCap(arms, recipe, limit, mode, capOverride)
	}
	if recipe.Abstention.Threshold == nil || math.IsNaN(*recipe.Abstention.Threshold) || math.IsInf(*recipe.Abstention.Threshold, 0) {
		return Response{}, errors.New("uncalibrated retrieval recipe")
	}
	cap, err := effectiveCap(capOverride, recipe.DocumentCap, limit)
	if err != nil {
		return Response{}, err
	}
	primary, err := rankWithSiblingPolicy(arms, recipe, limit, mode, cap, true)
	if err != nil {
		return Response{}, err
	}
	tail := recipe.NonCurrentTail
	if tailOverride != nil {
		tail = *tailOverride
		arms.IncludeNonCurrent = false
	}
	if tail < 0 || tail > 5 {
		return Response{}, errors.New("invalid non-current tail")
	}
	tail = min(tail, limit/2)
	if arms.IncludeNonCurrent {
		tail = limit / 2
	}
	other := Ranking{}
	if tail > 0 || arms.IncludeNonCurrent {
		other, err = rankGroupWithSiblingPolicy(arms.NonCurrent, arms.Validation.Routed, recipe, limit, mode, cap, true)
		if err != nil {
			return Response{}, err
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
	plan := responsePlan{response: out, primary: primary.Groups[:primaryCount], nonCurrent: other.Groups[:count]}
	return m.shapePlan(arms, recipe, plan)
}

// ShapeGroups shapes every frozen primary group before document and output caps.
// Evaluation uses it to distinguish evidence loss from a later ranking loss.
func (m *SiblingMerge) ShapeGroups(arms Arms, recipe Recipe, limit int, mode ArmMode) ([]Hit, error) {
	if limit == 0 {
		limit = 8
	}
	ranking, err := rankWithSiblingPolicy(arms, recipe, limit, mode, recipe.DocumentCap, true, true)
	if err != nil {
		return nil, err
	}
	out, err := m.shapePlan(arms, recipe, responsePlan{primary: ranking.Groups})
	if err != nil {
		return nil, err
	}
	return out.Hits, nil
}

func (m *SiblingMerge) shapePlan(arms Arms, recipe Recipe, plan responsePlan) (Response, error) {
	// Copy node maps because the same cache also serves unmerged trials.
	nodes := make(map[int64][]string, len(arms.EffectiveNodes))
	for k, v := range arms.EffectiveNodes {
		nodes[k] = v
	}
	refs := make(map[string]NodeRef, len(arms.Nodes))
	for k, v := range arms.Nodes {
		refs[k] = v
	}
	if m != nil {
		for k, v := range m.EffectiveNodes {
			nodes[k] = v
		}
		for k, v := range m.Nodes {
			refs[k] = v
		}
	}
	arms.EffectiveNodes, arms.Nodes = nodes, refs
	for _, groups := range [][]Group{plan.primary, plan.nonCurrent} {
		for i := range groups {
			g := &groups[i]
			if len(g.siblings) == 0 {
				continue
			}
			article, _ := groupArticle(*g)
			if m == nil {
				return Response{}, ErrCacheInsufficient
			}
			c, ok := m.Articles[article.ID]
			if !ok {
				return Response{}, ErrCacheInsufficient
			}
			g.article = &c
		}
	}
	return plan.shape(arms, recipe)
}
