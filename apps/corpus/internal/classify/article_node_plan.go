package classify

import (
	"context"
	"fmt"
	"sort"
)

func buildArticleNodePlan(ctx context.Context, r Recipe, articles []Article, choices map[string]Outcome, nodes []Node, count TokenCounter) (ArticlePlan, error) {
	var plan ArticlePlan
	seen := map[string]bool{}
	for start := 0; start < len(articles); {
		a := articles[start]
		end := start + 1
		for end < len(articles) && articles[end].DocumentKey == a.DocumentKey && articles[end].Chapter == a.Chapter {
			end++
		}
		group := articles[start:end]
		byFamily := map[string][]Article{}
		for _, article := range group {
			key := article.DocumentKey + "/" + article.Anchor
			choice, ok := choices[key]
			if !ok || seen[key] || article.DocumentKey == "" || article.Anchor == "" {
				return plan, fmt.Errorf("step-1 artifact omits or duplicates a selected article")
			}
			seen[key] = true
			if choice.Outcome != "ok" || len(choice.Codes) == 0 {
				status := "invalid_response"
				if choice.Outcome == "no_family" || choice.Outcome == "ok" {
					status = "no_family"
				}
				plan.Skipped = append(plan.Skipped, ArticleRecord{article.DocumentKey, article.Anchor, emptyArticle(status)})
				continue
			}
			familySeen := map[string]bool{}
			for _, code := range choice.Codes {
				if familySeen[code] {
					return plan, fmt.Errorf("duplicate step-1 family")
				}
				familySeen[code] = true
				byFamily[code] = append(byFamily[code], article)
			}
		}
		families := make([]string, 0, len(byFamily))
		for family := range byFamily {
			families = append(families, family)
		}
		sort.Strings(families)
		for _, family := range families {
			menu, err := allowedNodes([]string{family}, nodes)
			if err != nil {
				return plan, err
			}
			var familyPlan ArticlePlan
			selected := byFamily[family]
			for len(selected) > 0 {
				size := len(selected)
				if a.Chapter == "" {
					size = min(size, 12)
				}
				if err = splitArticles(ctx, r, selected[:size], menu, nil, count, &familyPlan); err != nil {
					return plan, err
				}
				selected = selected[size:]
			}
			for i := range familyPlan.Skipped {
				failed := 1
				familyPlan.Skipped[i].Outcome.FailedFamilies = &failed
			}
			plan.Requests = append(plan.Requests, familyPlan.Requests...)
			plan.Skipped = append(plan.Skipped, familyPlan.Skipped...)
		}
		start = end
	}
	return plan, nil
}
