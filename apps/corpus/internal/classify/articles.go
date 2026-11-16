package classify

import (
	"context"
	"encoding/json"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/batchapi"
)

type articleWork struct {
	articles   []Article
	families   FamilyArtifact
	nodes      []Node
	labelsHash string
}

func articleWorkHash(o Options) string {
	type selected struct{ DocumentKey, Anchor, Heading, Text, Chapter string }
	selection := make([]selected, 0, len(o.article.articles))
	for _, a := range o.article.articles {
		selection = append(selection, selected{a.DocumentKey, a.Anchor, a.Heading, a.Text, a.Chapter})
	}
	b, _ := json.Marshal(struct {
		RunID                                    int64
		Country, Release, RecipeHash, LabelsHash string
		Articles                                 []selected
	}{o.RunID, o.Country, o.Release, o.Recipe.Hash(), o.article.labelsHash, selection})
	return digest(b)
}

func (r *Runner) Articles(ctx context.Context, o Options, labelsHash string, articles []Article, families FamilyArtifact, nodes []Node) (Summary, error) {
	if o.Recipe.Article == nil || o.Recipe.Validate() != nil || o.Recipe.Article.SourceRecipeHash != families.RecipeHash || families.RecipeHash != families.Recipe.Hash() || families.TaxonomyHash != o.Recipe.TaxonomyHash || len(articles) == 0 {
		return Summary{}, fmt.Errorf("article recipe, families artifact and selected articles must agree")
	}
	o.Documents = 1
	o.article = &articleWork{articles: articles, families: families, nodes: nodes, labelsHash: labelsHash}
	return r.screen(ctx, o, nil, nil, nil)
}

func (r *Runner) prepareArticleBatch(ctx context.Context, o Options) (*screenBatch, error) {
	w := o.article
	plan, err := BuildArticlePlan(ctx, o.Recipe, w.articles, w.families.Documents, w.nodes, r.Client.CountTokens)
	if err != nil {
		return nil, err
	}
	s := Submission{RunID: o.RunID, Country: o.Country, Release: o.Release, ScreenHash: screenHash(o), Recipe: o.Recipe, State: "preparing", CreatedAt: r.Now(), Budget: o.Budget, BudgetMicroUSD: o.BudgetMicroUSD, Results: map[string]Outcome{}}
	meta := &ArticleMetadata{LabelsHash: w.labelsHash, Skipped: plan.Skipped}
	var requests []batchapi.Request
	size := len(`{"requests":[]}`)
	var summary Summary
	for i, req := range plan.Requests {
		p, buildErr := BuildArticleRequest(o.Recipe, req)
		if buildErr != nil {
			return nil, buildErr
		}
		reserve, reserveErr := o.Recipe.Reserve(req.InputTokens)
		if reserveErr != nil {
			return nil, reserveErr
		}
		s.ReserveMicroUSD += reserve
		summary.InputTokens += req.InputTokens
		b, encodeErr := json.Marshal(p)
		if encodeErr != nil {
			return nil, fmt.Errorf("encode article request: %w", encodeErr)
		}
		item := Item{Key: fmt.Sprintf("%s/%s", req.DocumentKey, req.Articles[0].Anchor), RequestHash: digest(b), InputTokens: req.InputTokens, Article: &req}
		if i == 0 {
			item.ArticleMeta = meta
		}
		s.Items = append(s.Items, item)
		request := batchapi.Request{CustomID: customID("00000000000000000000000000000000", i), Params: p}
		encoded, encodeErr := json.Marshal(batchapi.Parameters(request))
		if encodeErr != nil {
			return nil, fmt.Errorf("encode article batch request: %w", encodeErr)
		}
		size += len(encoded) + 1
		requests = append(requests, request)
	}
	if len(requests) > 100000 || size > 256000000 {
		return nil, fmt.Errorf("article batch exceeds provider limit")
	}
	if len(requests) == 0 {
		s.Items = []Item{{ArticleMeta: meta}}
	}
	summary.Requests = len(requests)
	summary.ReserveMicroUSD = s.ReserveMicroUSD
	return &screenBatch{submission: s, requests: requests, summary: summary}, nil
}
