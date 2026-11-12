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

func rankWithCap(arms Arms, recipe Recipe, limit int, mode ArmMode, cap int) (Ranking, error) {
	if limit == 0 {
		limit = 8
	}
	if limit < 1 || limit > 100 || (mode != "" && mode != "hybrid" && mode != "dense" && mode != "lexical") || recipe.RRF.K < 1 || recipe.HydrateMultiplier < 1 || recipe.DocumentCap < 1 || recipe.NonCurrentTail < 0 || recipe.NonCurrentTail > 5 || cap < 1 {
		return Ranking{}, errors.New("invalid ranking options")
	}
	if (mode != "lexical" && arms.Depths.Dense < recipe.DenseDepth) || (mode != "dense" && arms.Depths.Lexical < recipe.LexicalDepth) {
		return Ranking{}, ErrCacheInsufficient
	}
	return rankGroup(arms.Primary, arms.Validation.Routed, recipe, limit, mode, cap)
}

func rankGroup(arm ArmGroup, routed bool, recipe Recipe, limit int, mode ArmMode, cap int) (Ranking, error) {
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
	var first *float64
	if len(groups) > 0 {
		v := groups[0].BestCosine
		first = &v
	}
	counts := map[string]int{}
	selected := []Group{}
	demoted := []Group{}
	for _, g := range groups {
		key := g.Identity.DocumentKey
		if counts[key] < cap {
			selected = append(selected, g)
			counts[key]++
		} else {
			demoted = append(demoted, g)
		}
	}
	selected = append(selected, demoted...)
	selected = selected[:min(limit, len(selected))]
	return Ranking{Groups: selected, FirstBestCosine: first}, nil
}
