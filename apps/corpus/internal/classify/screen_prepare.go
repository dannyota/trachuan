package classify

import (
	"context"
	"encoding/json"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/batchapi"
)

type screenBatch struct {
	submission Submission
	requests   []batchapi.Request
	summary    Summary
}

func (r *Runner) prepareScreenBatch(ctx context.Context, o Options, docs []Document, families []Family) (*screenBatch, error) {
	var summary Summary
	selected, err := draw(docs, o.Documents, o.Seed)
	if err != nil {
		return nil, err
	}
	s := Submission{RunID: o.RunID, Country: o.Country, Release: o.Release, ScreenHash: screenHash(o), Recipe: o.Recipe, Families: families, State: "preparing", CreatedAt: r.Now(), Budget: o.Budget, BudgetMicroUSD: o.BudgetMicroUSD, Results: map[string]Outcome{}}
	requests := make([]batchapi.Request, len(selected))
	bytes := len(`{"requests":[]}`)
	for i, d := range selected {
		p, buildErr := BuildRequest(o.Recipe, d, families)
		if buildErr != nil {
			return nil, buildErr
		}
		n, countErr := r.Client.CountTokens(ctx, p)
		if countErr != nil {
			return nil, providerError(countErr)
		}
		reserve, reserveErr := o.Recipe.Reserve(n)
		if reserveErr != nil {
			return nil, reserveErr
		}
		s.ReserveMicroUSD += reserve
		summary.InputTokens += n
		b, marshalErr := json.Marshal(p)
		if marshalErr != nil {
			return nil, fmt.Errorf("encode counted request: %w", marshalErr)
		}
		s.Items = append(s.Items, Item{Key: d.Key, RequestHash: digest(b), InputTokens: n})
		requests[i] = batchapi.Request{CustomID: customID("00000000000000000000000000000000", i), Params: p}
		encoded, encodeErr := json.Marshal(batchapi.Parameters(requests[i]))
		if encodeErr != nil {
			return nil, fmt.Errorf("encode batch request: %w", encodeErr)
		}
		bytes += len(encoded) + 1
	}
	// Anthropic limits batches to 256 MB: https://platform.claude.com/docs/en/build-with-claude/batch-processing
	if len(requests) > 100000 || bytes > 256000000 {
		return nil, fmt.Errorf("classification batch exceeds provider limit")
	}
	summary.Requests = len(requests)
	summary.ReserveMicroUSD = s.ReserveMicroUSD
	return &screenBatch{submission: s, requests: requests, summary: summary}, nil
}
