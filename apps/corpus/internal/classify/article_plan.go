package classify

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

type TokenCounter func(context.Context, anthropic.MessageNewParams) (int64, error)

func BuildArticlePlan(ctx context.Context, r Recipe, articles []Article, families map[string]Outcome, nodes []Node, count TokenCounter) (ArticlePlan, error) {
	var plan ArticlePlan
	if err := r.Validate(); err != nil {
		return plan, err
	}
	if r.Article == nil || len(articles) == 0 {
		return plan, fmt.Errorf("article recipe and selected articles are required")
	}
	seen := map[string]bool{}
	for start := 0; start < len(articles); {
		a := articles[start]
		end := start + 1
		for end < len(articles) && articles[end].DocumentKey == a.DocumentKey && articles[end].Chapter == a.Chapter && (a.Chapter != "" || end-start < 12) {
			end++
		}
		group := articles[start:end]
		for _, v := range group {
			key := v.DocumentKey + "/" + v.Anchor
			if seen[key] || v.DocumentKey == "" || v.Anchor == "" {
				return plan, fmt.Errorf("empty or duplicate selected article")
			}
			seen[key] = true
		}
		family, ok := families[a.DocumentKey]
		if !ok {
			return plan, fmt.Errorf("selected document missing from families artifact")
		}
		if family.Outcome != "ok" || len(family.Codes) == 0 {
			outcome := "invalid_response"
			if family.Outcome == "no_family" || family.Outcome == "ok" {
				outcome = "no_family"
			}
			for _, v := range group {
				plan.Skipped = append(plan.Skipped, ArticleRecord{v.DocumentKey, v.Anchor, emptyArticle(outcome)})
			}
		} else {
			menu, err := allowedNodes(family.Codes, nodes)
			if err != nil {
				return plan, err
			}
			if err = splitArticles(ctx, r, group, menu, count, &plan); err != nil {
				return plan, err
			}
		}
		start = end
	}
	return plan, nil
}

func allowedNodes(codes []string, nodes []Node) ([]Node, error) {
	byCode := map[string]Node{}
	for _, n := range nodes {
		byCode[n.Code] = n
	}
	families := map[string]bool{}
	for _, code := range codes {
		n, ok := byCode[code]
		if !ok || !n.Active || n.Level != 2 {
			return nil, fmt.Errorf("family artifact names an inactive or non-family node")
		}
		families[code] = true
	}
	var menu []Node
	for _, n := range nodes {
		if !n.Active || n.Level < 2 || n.Level > 4 || (n.Origin != "phapdien" && n.Origin != "custom") {
			continue
		}
		p := n
		for steps := 0; steps < 4 && p.Level > 2; steps++ {
			p = byCode[p.Parent]
		}
		if families[p.Code] {
			menu = append(menu, n)
		}
	}
	return menu, nil
}

func splitArticles(ctx context.Context, r Recipe, group []Article, menu []Node, count TokenCounter, plan *ArticlePlan) error {
	var current *ArticleRequest
	for _, a := range group {
		candidate := ArticleRequest{DocumentKey: a.DocumentKey, Articles: []Article{a}, Nodes: menu}
		if current != nil {
			candidate.Articles = append(append([]Article(nil), current.Articles...), a)
		}
		n, err := countArticle(ctx, r, candidate, count)
		if err != nil {
			return err
		}
		if n > r.InputBound && current != nil {
			plan.Requests = append(plan.Requests, *current)
			current = nil
			candidate.Articles = []Article{a}
			n, err = countArticle(ctx, r, candidate, count)
			if err != nil {
				return err
			}
		}
		if n > r.InputBound {
			plan.Skipped = append(plan.Skipped, ArticleRecord{a.DocumentKey, a.Anchor, emptyArticle("too_long")})
			continue
		}
		candidate.InputTokens = n
		current = &candidate
	}
	if current != nil {
		plan.Requests = append(plan.Requests, *current)
	}
	return nil
}

func countArticle(ctx context.Context, r Recipe, req ArticleRequest, count TokenCounter) (int64, error) {
	p, err := BuildArticleRequest(r, req)
	if err != nil {
		return 0, err
	}
	n, err := count(ctx, p)
	if err != nil {
		return 0, providerError(err)
	}
	if n < 1 {
		return 0, fmt.Errorf("article token count must be positive")
	}
	return n, nil
}

func ValidateArticleFamilies(articles []Article, families FamilyArtifact) error {
	for _, a := range articles {
		if _, ok := families.Documents[a.DocumentKey]; !ok {
			return fmt.Errorf("selected document %s missing from families artifact", a.DocumentKey)
		}
	}
	return nil
}
