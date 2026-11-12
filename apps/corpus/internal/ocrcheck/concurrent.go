package ocrcheck

import (
	"context"
	"fmt"
	"sync"
)

// runConcurrent keeps selection, reservations, durable writes and summary updates
// under one mutex. Rendering, counting and generation run outside that section.
func (r *Runner) runConcurrent(ctx context.Context, docs []Document, o Options, s *Summary, skip map[int64]bool, ceiling int64) error {
	var mu sync.Mutex
	var stopped error
	next, selected := 0, 0
	take := func() (Document, bool) {
		mu.Lock()
		defer mu.Unlock()
		for stopped == nil && next < len(docs) {
			if ctx.Err() != nil {
				stopped = safeError(ctx.Err())
				break
			}
			doc := docs[next]
			next++
			if skip[doc.Entry.ID] {
				continue
			}
			skip[doc.Entry.ID] = true
			complete, err := r.Repository.Completed(ctx, doc.Entry.ID, o.CheckerID())
			if err != nil {
				stopped = fmt.Errorf("read OCR check state: %w", err)
				break
			}
			if complete {
				continue
			}
			if !o.RetryFailed {
				count, preRequest, err := r.Repository.RetryState(ctx, doc.Entry.ID, o.CheckerID())
				if err != nil {
					stopped = fmt.Errorf("read OCR retry state: %w", err)
					break
				}
				if count >= maxBilledFailures || preRequest {
					s.SkippedRetryLimit++
					s.record(doc, 0, Usage{}, "retry_limit")
					continue
				}
			}
			if o.LimitDocs > 0 && selected >= o.LimitDocs {
				return Document{}, false
			}
			selected++
			return doc, true
		}
		return Document{}, false
	}
	stop := func(err error) {
		if err != nil && stopped == nil {
			stopped = err
		}
	}
	failed := func(p prepared, reason string, u Usage) {
		mu.Lock()
		defer mu.Unlock()
		if !o.DryRun {
			if err := r.failAttempt(ctx, p.doc, o, reason, u); err != nil {
				stop(err)
				return
			}
		}
		s.Failed++
		s.record(p.doc, p.pages, u, reason)
	}
	price, _ := modelPrice(o.Model, false)
	worker := func() {
		for {
			doc, ok := take()
			if !ok {
				return
			}
			ready, reason := r.prepare(ctx, doc, o)
			if reason != "" {
				failed(ready, reason, Usage{PDFPages: ready.pages, Rendering: ready.rendering})
				continue
			}
			tokens, err := r.Client.CountTokens(ctx, ready.request)
			if err != nil || tokens < 0 || tokens > 1_000_000_000 {
				u := errorUsage(err)
				u.PDFPages = ready.pages
				u.Rendering = ready.rendering
				failed(ready, errorReason(err), u)
				if o.DryRun {
					mu.Lock()
					stop(safeError(err))
					mu.Unlock()
				}
				continue
			}
			if tokens+maxOutputTokens > 1_000_000 {
				failed(ready, "over_check_limit", Usage{PDFPages: ready.pages, Rendering: ready.rendering})
				continue
			}
			ready.inputTokens = tokens
			ready.worst = price.cost(Usage{Input: tokens, Output: maxOutputTokens})
			mu.Lock()
			if stopped != nil {
				mu.Unlock()
				return
			}
			if ctx.Err() != nil {
				stop(safeError(ctx.Err()))
				mu.Unlock()
				return
			}
			// CostMicroUSD includes both real charges and all outstanding holds.
			if ready.worst > ceiling-s.CostMicroUSD-s.EstimateMicroUSD {
				stop(ErrBudget)
				mu.Unlock()
				return
			}
			if o.DryRun {
				s.Estimated++
				s.EstimateMicroUSD += ready.worst
				s.record(doc, ready.pages, Usage{Input: tokens, CostMicroUSD: ready.worst}, "estimated")
				mu.Unlock()
				continue
			}
			attempts, err := r.begin(ctx, []prepared{ready}, o)
			if err != nil {
				stop(err)
				mu.Unlock()
				return
			}
			a := attempts[0]
			s.CostMicroUSD += a.Usage.charge()
			mu.Unlock()
			response, callErr := r.Client.Direct(ctx, ready.request)
			mu.Lock()
			if !response.FinalUsage && !preGenerationFailure(response, callErr) {
				stop(r.unknown(ctx, a, doc, response, callErr, o, s))
			} else {
				if !response.FinalUsage {
					response.Usage = errorUsage(callErr)
				}
				stop(r.finish(ctx, a, ready, response, callErr, o, s))
			}
			mu.Unlock()
		}
	}
	workers := 8
	if o.DryRun {
		workers = 1
	}
	var wg sync.WaitGroup
	for range workers {
		wg.Go(worker)
	}
	wg.Wait()
	return stopped
}
