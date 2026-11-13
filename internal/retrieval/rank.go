package retrieval

import (
	"errors"
	"math"
	"sort"
)

// ErrCacheInsufficient indicates the requested ranking exceeds the cached arms.
var ErrCacheInsufficient = errors.New("cache_insufficient")

// Group holds the highest-scoring member and all cached members of a provision.
type Group struct {
	Identity   Identity
	Score      float64
	Best       Candidate
	Members    []Candidate
	BestCosine float64
	siblings   []Group
	article    *Candidate
}

// Ranking holds ranked provision groups and the first group's confidence.
type Ranking struct {
	Groups          []Group
	FirstBestCosine *float64
}

type fusedCandidate struct {
	candidate Candidate
	score     float64
}

// Rank fuses candidate arms and ranks provision groups without a confidence gate.
func Rank(arms Arms, recipe Recipe, limit int, mode ArmMode, overrides ...*CapOverride) (Ranking, error) {
	if limit == 0 {
		limit = 8
	}
	if limit < 1 || limit > MaxLimit {
		return Ranking{}, errors.New("invalid ranking options")
	}
	cap := recipe.DocumentCap
	if len(overrides) > 1 {
		return Ranking{}, errors.New("invalid ranking options")
	}
	if len(overrides) == 1 {
		var err error
		cap, err = effectiveCap(overrides[0], cap, limit)
		if err != nil {
			return Ranking{}, err
		}
	}
	return rankWithCap(arms, recipe, limit, mode, cap)
}

type rankPolicy struct {
	merge, allGroups bool
	article          *ArticleCapOverride
}

func rankWithCap(arms Arms, recipe Recipe, limit int, mode ArmMode, cap int, article ...*ArticleCapOverride) (Ranking, error) {
	policy := rankPolicy{}
	if len(article) > 0 {
		policy.article = article[0]
	}
	return rankWithPolicy(arms, recipe, limit, mode, cap, policy)
}

func rankWithSiblingPolicy(arms Arms, recipe Recipe, limit int, mode ArmMode, cap int, merge bool, allGroups ...bool) (Ranking, error) {
	return rankWithPolicy(arms, recipe, limit, mode, cap, rankPolicy{merge: merge, allGroups: len(allGroups) > 0 && allGroups[0]})
}

func rankWithPolicy(arms Arms, recipe Recipe, limit int, mode ArmMode, cap int, policy rankPolicy) (Ranking, error) {
	if limit == 0 {
		limit = 8
	}
	if limit < 1 || limit > 100 || (mode != "" && mode != "hybrid" && mode != "dense" && mode != "lexical") || recipe.RRF.K < 1 || recipe.HydrateMultiplier < 1 || recipe.DocumentCap < 1 || recipe.NonCurrentTail < 0 || recipe.NonCurrentTail > 5 || cap < 1 {
		return Ranking{}, errors.New("invalid ranking options")
	}
	if (mode != "lexical" && arms.Depths.Dense < recipe.DenseDepth) || (mode != "dense" && arms.Depths.Lexical < recipe.LexicalDepth) {
		return Ranking{}, ErrCacheInsufficient
	}
	return rankGroupWithPolicy(arms.Primary, arms.Validation.Routed, recipe, limit, mode, cap, policy)
}

func rankGroup(arm ArmGroup, routed bool, recipe Recipe, limit int, mode ArmMode, cap int, article ...*ArticleCapOverride) (Ranking, error) {
	policy := rankPolicy{}
	if len(article) > 0 {
		policy.article = article[0]
	}
	return rankGroupWithPolicy(arm, routed, recipe, limit, mode, cap, policy)
}

func rankGroupWithSiblingPolicy(arm ArmGroup, routed bool, recipe Recipe, limit int, mode ArmMode, cap int, merge bool, allGroups ...bool) (Ranking, error) {
	return rankGroupWithPolicy(arm, routed, recipe, limit, mode, cap, rankPolicy{merge: merge, allGroups: len(allGroups) > 0 && allGroups[0]})
}

func rankGroupWithPolicy(arm ArmGroup, routed bool, recipe Recipe, limit int, mode ArmMode, cap int, policy rankPolicy) (Ranking, error) {
	scores := map[int64]fusedCandidate{}
	add := func(cs []Candidate, depth int, weight float64) {
		for i, c := range cs {
			if i >= depth {
				break
			}
			f := scores[c.ChunkID]
			f.candidate = c
			f.score += weight / float64(recipe.RRF.K+i+1)
			scores[c.ChunkID] = f
		}
	}
	if mode != "lexical" {
		add(arm.Dense, recipe.DenseDepth, recipe.RRF.DenseWeight)
	}
	if mode != "dense" {
		w := recipe.RRF.LexicalWeight
		if routed {
			w = recipe.RRF.RoutedLexicalWeight
		}
		add(arm.Lexical, recipe.LexicalDepth, w)
	}
	fused := make([]fusedCandidate, 0, len(scores))
	for _, f := range scores {
		fused = append(fused, f)
	}
	sort.Slice(fused, func(i, j int) bool {
		if fused[i].score != fused[j].score {
			return fused[i].score > fused[j].score
		}
		return fused[i].candidate.ChunkID < fused[j].candidate.ChunkID
	})
	fused = fused[:min(len(fused), recipe.HydrateMultiplier*limit)]
	groups := []Group{}
	indices := map[int64]int{}
	for _, f := range fused {
		c := f.candidate
		if _, ok := indices[c.GroupID]; ok {
			continue
		}
		indices[c.GroupID] = len(groups)
		groups = append(groups, Group{Identity: c.Group, Score: f.score, Best: c, BestCosine: math.Inf(-1)})
	}
	cached := map[int64]int64{}
	for _, c := range arm.Members {
		cached[c.ChunkID] = c.GroupID
	}
	for _, f := range fused {
		if group, ok := cached[f.candidate.ChunkID]; !ok || group != f.candidate.GroupID {
			return Ranking{}, ErrCacheInsufficient
		}
	}
	for _, c := range arm.Members {
		if i, ok := indices[c.GroupID]; ok {
			if math.IsNaN(c.Cosine) || math.IsInf(c.Cosine, 0) {
				return Ranking{}, errors.New("invalid candidate cosine")
			}
			groups[i].Members = append(groups[i].Members, c)
			groups[i].BestCosine = max(groups[i].BestCosine, max(-1, min(1, c.Cosine)))
		}
	}
	for _, g := range groups {
		if len(g.Members) == 0 {
			return Ranking{}, ErrCacheInsufficient
		}
	}
	if policy.merge {
		groups = mergeSiblingGroups(groups)
	}
	var first *float64
	if len(groups) > 0 {
		v := groups[0].BestCosine
		first = &v
	}
	if policy.allGroups {
		return Ranking{Groups: groups, FirstBestCosine: first}, nil
	}
	counts := map[string]int{}
	articleCounts := map[Identity]int{}
	selected := []Group{}
	demoted := []Group{}
	for _, g := range groups {
		key := g.Identity.DocumentKey
		var articleKey Identity
		hasArticle := false
		if policy.article != nil && *policy.article > 0 {
			for i := len(g.Best.Ancestors) - 1; i >= 0; i-- {
				p := g.Best.Ancestors[i]
				if p.Kind == "dieu" {
					articleKey, hasArticle = p.Identity, true
					break
				}
			}
		}
		if counts[key] < cap && (!hasArticle || articleCounts[articleKey] < int(*policy.article)) {
			selected = append(selected, g)
			counts[key]++
			if hasArticle {
				articleCounts[articleKey]++
			}
		} else {
			demoted = append(demoted, g)
		}
	}
	selected = append(selected, demoted...)
	selected = selected[:min(limit, len(selected))]
	return Ranking{Groups: selected, FirstBestCosine: first}, nil
}

// RankWithSiblingAndArticleCap applies the sibling merge and both research caps.
func RankWithSiblingAndArticleCap(arms Arms, recipe Recipe, limit int, mode ArmMode, document *CapOverride, article *ArticleCapOverride) (Ranking, error) {
	if limit == 0 {
		limit = 8
	}
	cap, err := effectiveCap(document, recipe.DocumentCap, limit)
	if err != nil {
		return Ranking{}, err
	}
	if err = validateArticleCap(article); err != nil {
		return Ranking{}, err
	}
	return rankWithPolicy(arms, recipe, limit, mode, cap, rankPolicy{merge: true, article: article})
}

// ShapeWithArticleCap applies both research caps after freezing sibling membership.
func (m *SiblingMerge) ShapeWithArticleCap(arms Arms, recipe Recipe, limit int, mode ArmMode, capOverride *CapOverride, tailOverride *int, article *ArticleCapOverride) (Response, error) {
	if err := validateArticleCap(article); err != nil {
		return Response{}, err
	}
	if article == nil || *article == 0 {
		return m.Shape(arms, recipe, limit, mode, capOverride, tailOverride)
	}
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
	primary, err := rankWithPolicy(arms, recipe, limit, mode, cap, rankPolicy{merge: true, article: article})
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
		other, err = rankGroupWithPolicy(arms.NonCurrent, arms.Validation.Routed, recipe, limit, mode, cap, rankPolicy{merge: true, article: article})
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
