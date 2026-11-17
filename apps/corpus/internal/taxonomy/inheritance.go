package taxonomy

import (
	"sort"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

type InheritanceArticle struct {
	ID, DocumentID  int64
	Anchor, Heading string
	Consolidated    bool
	Seeds           []string
}

func InheritanceCandidates(articles []InheritanceArticle, sources map[int64][]int64) map[int64][]string {
	byDocument := map[int64]map[string]InheritanceArticle{}
	for _, article := range articles {
		if byDocument[article.DocumentID] == nil {
			byDocument[article.DocumentID] = map[string]InheritanceArticle{}
		}
		byDocument[article.DocumentID][article.Anchor] = article
	}
	result := map[int64][]string{}
	for _, article := range articles {
		if !article.Consolidated || len(article.Seeds) != 0 || !numericAnchor(article.Anchor) {
			continue
		}
		heading := inheritanceHeading(article.Heading)
		if heading == "" {
			continue
		}
		codes := map[string]bool{}
		for _, document := range sources[article.DocumentID] {
			source, ok := byDocument[document][article.Anchor]
			if !ok || inheritanceHeading(source.Heading) != heading {
				continue
			}
			for _, code := range source.Seeds {
				codes[code] = true
			}
		}
		if len(codes) == 0 {
			continue
		}
		for code := range codes {
			result[article.ID] = append(result[article.ID], code)
		}
		sort.Strings(result[article.ID])
	}
	return result
}

func inheritanceHeading(s string) string {
	return strings.Join(strings.Fields(cases.Fold().String(norm.NFC.String(s))), " ")
}

func numericAnchor(anchor string) bool {
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
