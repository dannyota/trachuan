package classify

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"sort"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/batchapi"
)

func draw(docs []Document, n int, seed int64) ([]Document, error) {
	if len(docs) < n {
		return nil, fmt.Errorf("requested %d documents, only %d eligible", n, len(docs))
	}
	all := append([]Document(nil), docs...)
	sort.Slice(all, func(i, j int) bool { return all[i].Key < all[j].Key })
	for i := 1; i < len(all); i++ {
		if all[i].Key == all[i-1].Key {
			return nil, fmt.Errorf("duplicate eligible document")
		}
	}
	rng := rand.New(rand.NewPCG(uint64(seed), 0))
	rng.Shuffle(len(all), func(i, j int) { all[i], all[j] = all[j], all[i] })
	return all[:n], nil
}

func screenHash(o Options) string {
	b, _ := json.Marshal(struct {
		RunID            int64
		Country, Release string
		Documents        int
		Seed             int64
		RecipeHash       string
	}{o.RunID, o.Country, o.Release, o.Documents, o.Seed, o.Recipe.Hash()})
	return digest(b)
}

func (r *Runner) Screen(ctx context.Context, o Options, docs []Document, families []Family) (Summary, error) {
	var summary Summary
	if err := o.validate(); err != nil {
		return summary, err
	}
	if r.Now == nil {
		r.Now = time.Now
	}
	if r.Wait == nil {
		r.Wait = wait
	}
	unlock, err := r.Repository.Lock(ctx)
	if err != nil {
		return summary, err
	}
	defer unlock()
	rows, err := r.Repository.List(ctx)
	if err != nil {
		return summary, err
	}
	hash := screenHash(o)
	for _, s := range rows {
		if s.ScreenHash == hash && (s.State == "completed" || s.State == "cancelled") {
			summary = Summary{Requests: len(s.Items), ReserveMicroUSD: s.ReserveMicroUSD, Admitted: true, SubmissionID: s.ID}
			for _, item := range s.Items {
				summary.InputTokens += item.InputTokens
			}
			if o.DryRun {
				return summary, nil
			}
			return summary, writeFamilies(o.Out, s)
		}
	}
	for i := range rows {
		s := &rows[i]
		if s.State == "preparing" || s.State == "uncertain" {
			known, restoreErr := r.restoreReceipt(s)
			if restoreErr != nil {
				return summary, restoreErr
			}
			if known && !o.DryRun {
				if err = r.Repository.Save(ctx, *s); err != nil {
					return summary, err
				}
			}
		}
		if s.State == "preparing" && r.Now().Sub(s.CreatedAt) > 10*time.Minute {
			s.State = "uncertain"
			if !o.DryRun {
				if err = r.Repository.Save(ctx, *s); err != nil {
					return summary, err
				}
			}
		}
		if s.State == "uncertain" {
			return summary, fmt.Errorf("uncertain classification submission %s blocks new work; reconcile it with corpus submissions", s.ID)
		}
	}
	for i := len(rows) - 1; i >= 0; i-- {
		s := rows[i]
		if s.ScreenHash == hash && s.State != "abandoned" {
			summary = Summary{Requests: len(s.Items), ReserveMicroUSD: s.ReserveMicroUSD, Admitted: true, SubmissionID: s.ID}
			for _, item := range s.Items {
				summary.InputTokens += item.InputTokens
			}
			if o.DryRun {
				return summary, nil
			}
			if s.State == "completed" || s.State == "cancelled" {
				return summary, writeFamilies(o.Out, s)
			}
			if s.State != "submitted" && s.State != "cancelling" {
				return summary, fmt.Errorf("submission %s is %s; operator review required", s.ID, s.State)
			}
			return summary, r.poll(ctx, &s, o.Out)
		}
		if s.RunID == o.RunID && nonterminal(s.State) {
			return summary, fmt.Errorf("earlier classification submission %s blocks this release", s.ID)
		}
	}
	entries, readErr := os.ReadDir(o.Out)
	if readErr != nil && !os.IsNotExist(readErr) {
		return summary, fmt.Errorf("inspect screening output: %w", readErr)
	}
	if len(entries) > 0 {
		return summary, fmt.Errorf("screening output directory is not empty")
	}
	selected, err := draw(docs, o.Documents, o.Seed)
	if err != nil {
		return summary, err
	}
	s := Submission{RunID: o.RunID, Country: o.Country, Release: o.Release, ScreenHash: hash, Recipe: o.Recipe, Families: families, State: "preparing", CreatedAt: r.Now(), Budget: o.Budget, BudgetMicroUSD: o.BudgetMicroUSD, Results: map[string]Outcome{}}
	requests := make([]batchapi.Request, len(selected))
	bytes := len(`{"requests":[]}`)
	for i, d := range selected {
		p, buildErr := BuildRequest(o.Recipe, d, families)
		if buildErr != nil {
			return summary, buildErr
		}
		n, countErr := r.Client.CountTokens(ctx, p)
		if countErr != nil {
			return summary, providerError(countErr)
		}
		reserve, reserveErr := o.Recipe.Reserve(n)
		if reserveErr != nil {
			return summary, reserveErr
		}
		s.ReserveMicroUSD += reserve
		summary.InputTokens += n
		b, marshalErr := json.Marshal(p)
		if marshalErr != nil {
			return summary, fmt.Errorf("encode counted request: %w", marshalErr)
		}
		s.Items = append(s.Items, Item{Key: d.Key, RequestHash: digest(b), InputTokens: n})
		requests[i] = batchapi.Request{CustomID: customID("00000000000000000000000000000000", i), Params: p}
		encoded, encodeErr := json.Marshal(batchapi.Parameters(requests[i]))
		if encodeErr != nil {
			return summary, fmt.Errorf("encode batch request: %w", encodeErr)
		}
		bytes += len(encoded) + 1
	}
	// Anthropic limits batches to 256 MB: https://platform.claude.com/docs/en/build-with-claude/batch-processing
	if len(requests) > 100000 || bytes > 256000000 {
		return summary, fmt.Errorf("classification batch exceeds provider limit")
	}
	total, err := ledgerCost(rows)
	if err != nil {
		return summary, err
	}
	summary.Requests = len(requests)
	summary.ReserveMicroUSD = s.ReserveMicroUSD
	summary.LedgerMicroUSD = total
	summary.Admitted = total <= o.BudgetMicroUSD && s.ReserveMicroUSD <= o.BudgetMicroUSD-total
	if o.DryRun {
		return summary, nil
	}
	if !summary.Admitted {
		return summary, fmt.Errorf("classification batch refused by %s budget", o.Budget)
	}
	if s.ID, err = newID(); err != nil {
		return summary, err
	}
	summary.SubmissionID = s.ID
	s.ItemsHash = itemsHash(s.Items)
	s.CreatedAt = r.Now().UTC().Truncate(time.Microsecond)
	if err = r.Repository.Save(ctx, s); err != nil {
		return summary, err
	}
	if err = r.writeIntent(s); err != nil {
		return summary, err
	}
	for i := range requests {
		requests[i].CustomID = customID(s.ID, i)
	}
	b, err := r.Client.Create(ctx, requests)
	if err != nil || b.ID == "" {
		s.State = "uncertain"
		if saveErr := r.Repository.Save(ctx, s); saveErr != nil {
			return summary, saveErr
		}
		return summary, fmt.Errorf("classification create response is uncertain; reconcile submission %s", s.ID)
	}
	if !b.CreatedAt.IsZero() {
		s.SubmittedAt = &b.CreatedAt
	}
	if err = r.attach(ctx, &s, b.ID); err != nil {
		return summary, err
	}
	return summary, r.poll(ctx, &s, o.Out)
}

func wait(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
