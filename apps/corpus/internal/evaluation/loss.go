package evaluation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"danny.vn/trachuan/internal/retrieval"
)

// CitationLoss identifies the first stage that prevented a citation's response.
type CitationLoss struct {
	CitationIndex int    `json:"citation_index"`
	Stage         string `json:"stage"`
	Rank          int    `json:"rank,omitempty"`
}
type fusionItem struct {
	candidate retrieval.Candidate
	score     float64
}

func diagnosticRank(cohort string, e Expectation, entry MapEntry, hits []retrieval.Hit) int {
	if cohort == "legacy" {
		rank, _ := MatchLegacy(e, hits)
		return rank
	}
	return MatchProduct(entry, hits)
}
func candidateHit(c retrieval.Candidate) retrieval.Hit {
	labels := []string{}
	path := []retrieval.PathNode{}
	for _, p := range c.Ancestors {
		path = append(path, retrieval.PathNode{Identity: p.Identity, Kind: p.Kind, Label: p.Label})
		if p.Label != "" {
			labels = append(labels, strings.TrimSpace(p.Label))
		}
	}
	h := retrieval.Hit{Identity: c.Identity, Path: path, Number: c.Document.Number, Citation: strings.Join(labels, ", ")}
	for _, p := range c.Descendants {
		h.Fragments = append(h.Fragments, retrieval.Fragment{Identity: p.Identity, Complete: true})
	}
	for _, r := range c.Relations {
		relation := r.Value
		sum := sha256.Sum256([]byte(relation.Evidence))
		relation.EvidenceSHA256 = hex.EncodeToString(sum[:])
		relation.Complete = true
		h.Relations = append(h.Relations, relation)
	}
	return h
}
func matchingCandidate(cohort string, e Expectation, entry MapEntry, candidates []retrieval.Candidate) bool {
	for _, c := range candidates {
		if diagnosticRank(cohort, e, entry, []retrieval.Hit{candidateHit(c)}) > 0 {
			return true
		}
	}
	return false
}
func fusedItems(arms retrieval.Arms, recipe retrieval.Recipe, mode retrieval.ArmMode) []fusionItem {
	scores := map[int64]fusionItem{}
	add := func(cs []retrieval.Candidate, depth int, weight float64) {
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
		add(arms.Primary.Dense, recipe.DenseDepth, recipe.RRF.DenseWeight)
	}
	if mode != "dense" {
		weight := recipe.RRF.LexicalWeight
		if arms.Validation.Routed {
			weight = recipe.RRF.RoutedLexicalWeight
		}
		add(arms.Primary.Lexical, recipe.LexicalDepth, weight)
	}
	items := make([]fusionItem, 0, len(scores))
	for _, v := range scores {
		items = append(items, v)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].score != items[j].score {
			return items[i].score > items[j].score
		}
		return items[i].candidate.ChunkID < items[j].candidate.ChunkID
	})
	return items
}
func groupMatches(arms retrieval.Arms, recipe retrieval.Recipe, candidate retrieval.Candidate, cohort string, e Expectation, entry MapEntry) (bool, error) {
	group := retrieval.ArmGroup{Dense: []retrieval.Candidate{candidate}}
	for _, m := range arms.Primary.Members {
		if m.GroupID == candidate.GroupID {
			group.Members = append(group.Members, m)
		}
	}
	probe := arms
	probe.Primary = group
	probe.IncludeNonCurrent = false
	recipe.NonCurrentTail = 0
	recipe.DenseDepth = 1
	recipe.LexicalDepth = 1
	response, err := retrieval.Shape(probe, recipe, 1, "dense")
	if err != nil {
		return false, fmt.Errorf("shape diagnostic group: %w", err)
	}
	return diagnosticRank(cohort, e, entry, response.Hits) > 0, nil
}
func diagnoseLoss(arms retrieval.Arms, recipe retrieval.Recipe, limit int, mode retrieval.ArmMode, cohort string, e Expectation, entry MapEntry, response retrieval.Response, tail *int, overrides ...any) (string, error) {
	articleCap, merge := diagnosticOverrides(overrides)
	hits := append(append([]retrieval.Hit{}, response.Hits...), response.NonCurrent...)
	rank := diagnosticRank(cohort, e, entry, hits)
	effectiveTail := min(recipe.NonCurrentTail, limit/2)
	if tail != nil {
		effectiveTail = min(*tail, limit/2)
	}
	includeComplement := (tail == nil && arms.IncludeNonCurrent) || effectiveTail > 0
	deep := append(append([]retrieval.Candidate{}, arms.Primary.Dense...), arms.Primary.Lexical...)
	if includeComplement {
		deep = append(deep, arms.NonCurrent.Dense...)
		deep = append(deep, arms.NonCurrent.Lexical...)
	}
	if !matchingCandidate(cohort, e, entry, deep) && rank == 0 {
		return "candidate", nil
	}
	items := fusedItems(arms, recipe, mode)
	primaryLimit := limit - len(response.NonCurrent)
	if tail == nil && arms.IncludeNonCurrent {
		primary, err := retrieval.RankWithArticleCap(arms, recipe, limit, mode, nil, articleCap)
		if err != nil {
			return "", fmt.Errorf("rank diagnostic primary: %w", err)
		}
		reserved := max(limit/2, limit-len(primary.Groups))
		primaryLimit = limit - min(reserved, len(response.NonCurrent))
	}
	shaped, rankErr := retrieval.RankWithArticleCap(arms, recipe, max(1, primaryLimit), mode, nil, articleCap)
	if rankErr != nil {
		return "", fmt.Errorf("rank diagnostic shaped primary: %w", rankErr)
	}
	if len(response.Hits) > len(shaped.Groups) {
		primaryLimit = limit
	}
	if merge != nil {
		primaryLimit = limit
	}
	items = items[:min(len(items), recipe.HydrateMultiplier*primaryLimit)]
	primaryCount := len(items)
	if includeComplement {
		other := arms
		other.Primary = arms.NonCurrent
		complement := fusedItems(other, recipe, mode)
		items = append(items, complement[:min(len(complement), recipe.HydrateMultiplier*limit)]...)
	}
	candidates := make([]retrieval.Candidate, 0, len(items))
	for _, v := range items {
		candidates = append(candidates, v.candidate)
	}
	if !matchingCandidate(cohort, e, entry, candidates) && rank == 0 {
		return "fusion", nil
	}
	groupMatch := false
	if merge != nil {
		var err error
		groupMatch, err = siblingGroupsMatch(merge, arms, recipe, limit, mode, cohort, e, entry, includeComplement)
		if err != nil {
			return "", err
		}
	} else {
		groups := map[[2]int64]bool{}
		for i, item := range items {
			c := item.candidate
			group := int64(0)
			probe := arms
			if i >= primaryCount {
				group = 1
				probe.Primary = arms.NonCurrent
			}
			key := [2]int64{group, c.GroupID}
			if groups[key] {
				continue
			}
			groups[key] = true
			match, err := groupMatches(probe, recipe, c, cohort, e, entry)
			if err != nil {
				return "", err
			}
			groupMatch = groupMatch || match
		}
	}
	if !groupMatch && rank == 0 {
		return "grouping", nil
	}
	uncapped := recipe
	uncapped.DocumentCap = limit + len(items)
	var alternate retrieval.Response
	var err error
	if merge != nil {
		alternate, err = merge.Shape(arms, uncapped, limit, mode, nil, tail)
	} else if tail != nil {
		alternate, err = retrieval.ShapeWithNonCurrentTail(arms, uncapped, limit, mode, nil, *tail)
	} else {
		alternate, err = retrieval.Shape(arms, uncapped, limit, mode)
	}
	if err != nil {
		return "", fmt.Errorf("shape uncapped diagnostics: %w", err)
	}
	altHits := append(append([]retrieval.Hit{}, alternate.Hits...), alternate.NonCurrent...)
	altRank := diagnosticRank(cohort, e, entry, altHits)
	if altRank > 0 && (rank == 0 || altRank < rank) {
		return "cap", nil
	}
	if rank == 0 {
		return "rank_limit", nil
	}
	if response.Abstain {
		return "abstention", nil
	}
	if rank > 1 {
		return "ranked", nil
	}
	return "", nil
}

func caseDiagnostics(arms retrieval.Arms, recipe retrieval.Recipe, limit int, mode retrieval.ArmMode, cohort string, expectations []Expectation, entries map[int]MapEntry, response retrieval.Response, tail *int, overrides ...any) ([]CitationLoss, []retrieval.Gap, error) {
	hits := append(append([]retrieval.Hit{}, response.Hits...), response.NonCurrent...)
	gaps := append([]retrieval.Gap{}, response.Gaps...)
	for _, h := range hits {
		gaps = append(gaps, h.Gaps...)
	}
	losses := []CitationLoss{}
	for i, e := range expectations {
		rank := diagnosticRank(cohort, e, entries[i], hits)
		if rank == 1 && !response.Abstain {
			continue
		}
		stage, err := diagnoseLoss(arms, recipe, limit, mode, cohort, e, entries[i], response, tail, overrides...)
		if err != nil {
			return nil, nil, fmt.Errorf("diagnose citation %d: %w", i, err)
		}
		if stage != "" {
			losses = append(losses, CitationLoss{CitationIndex: i, Stage: stage, Rank: rank})
		}
	}
	return losses, gaps, nil
}

func diagnosticOverrides(overrides []any) (*retrieval.ArticleCapOverride, *retrieval.SiblingMerge) {
	var article *retrieval.ArticleCapOverride
	var merge *retrieval.SiblingMerge
	for _, override := range overrides {
		switch value := override.(type) {
		case *retrieval.ArticleCapOverride:
			article = value
		case *retrieval.SiblingMerge:
			merge = value
		}
	}
	return article, merge
}
