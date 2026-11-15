package build

import (
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

type inheritanceArticle struct {
	id, doc         int64
	anchor, heading string
	consolidated    bool
	seeds           []int64
}

type inheritanceResult struct {
	assignments map[int64]map[int64]bool
	unmatched   int64
	gate        inheritanceGate
}

func headingKey(s string) string {
	return strings.Join(strings.Fields(cases.Fold().String(norm.NFC.String(s))), " ")
}

func numericArticleAnchor(anchor string) bool {
	number, ok := strings.CutPrefix(anchor, "dieu-")
	if !ok || number == "" {
		return false
	}
	for _, r := range number {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func inheritArticles(articles []inheritanceArticle, sources map[int64][]int64, nodes map[int64]taxonomyNode) inheritanceResult {
	byDoc := map[int64]map[string]inheritanceArticle{}
	for _, a := range articles {
		if byDoc[a.doc] == nil {
			byDoc[a.doc] = map[string]inheritanceArticle{}
		}
		byDoc[a.doc][a.anchor] = a
	}
	result := inheritanceResult{assignments: map[int64]map[int64]bool{}}
	result.gate.Rows = [3]inheritanceRow{{Name: "level_2", Floor: 0.95}, {Name: "level_3", Floor: 0.90}, {Name: "deepest", Floor: 0.85}}
	for _, a := range articles {
		if !a.consolidated {
			continue
		}
		inherited := map[int64]bool{}
		heading := headingKey(a.heading)
		if heading != "" && numericArticleAnchor(a.anchor) {
			for _, doc := range sources[a.doc] {
				source, ok := byDoc[doc][a.anchor]
				if !ok || headingKey(source.heading) != heading {
					continue
				}
				for _, seed := range source.seeds {
					inherited[seed] = true
				}
			}
		}
		if len(a.seeds) == 0 {
			if len(inherited) == 0 {
				result.unmatched++
			} else {
				result.assignments[a.id] = inherited
			}
			continue
		}
		if len(inherited) == 0 {
			continue
		}
		result.gate.Articles++
		predicted, truth := inheritanceLevels(inherited, nodes), inheritanceLevels(setOf(a.seeds), nodes)
		for i := range result.gate.Rows {
			for id := range predicted[i] {
				result.gate.Rows[i].Denominator++
				if truth[i][id] {
					result.gate.Rows[i].Numerator++
				}
			}
		}
	}
	finishInheritanceGate(&result.gate)
	return result
}

func setOf(ids []int64) map[int64]bool {
	result := map[int64]bool{}
	for _, id := range ids {
		result[id] = true
	}
	return result
}

func inheritanceLevels(ids map[int64]bool, nodes map[int64]taxonomyNode) [3]map[int64]bool {
	result := [3]map[int64]bool{{}, {}, {}}
	for id := range ids {
		result[2][id] = true
		for id != 0 {
			n := nodes[id]
			if n.level == 2 {
				result[0][id] = true
			}
			if n.level == 3 {
				result[1][id] = true
			}
			id = n.parent
		}
	}
	return result
}
