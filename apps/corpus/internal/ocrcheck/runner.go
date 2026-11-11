package ocrcheck

import (
	"context"
	"fmt"
	"math"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

// ErrBudget means no further paid request fits the authorized ceiling.
const ErrBudget budgetError = "OCR check would exceed --max-usd; stopped before sending"

type budgetError string

// Error reports that the next request exceeds the spend ceiling.
func (e budgetError) Error() string { return string(e) }

// Document binds a store entry to the run's fetched PDF and verified OCR text.
type Document struct {
	Entry        store.OcrText
	PDFPath      string
	Text         []byte
	ReadText     func() ([]byte, error)
	Key, PDFName string
}

// Attempt is durable intent before a paid request, or its terminal outcome.
type Attempt struct {
	ID, EntryID                                         int64
	Checker, Mode, SubmissionID, BatchID, State, Reason string
	CreatedAt                                           time.Time
	Usage                                               Usage
}

// Repository persists attempts and completed checks atomically.
type Repository interface {
	Open(context.Context, string) ([]Attempt, error)
	Spent(context.Context, string) (int64, error)
	Completed(context.Context, int64, string) (bool, error)
	Begin(context.Context, []Intent, string, string, string, time.Time) ([]Attempt, error)
	Submitted(context.Context, string, string) error
	Uncertain(context.Context, string) error
	Abandon(context.Context, string) error
	Finish(context.Context, Attempt, store.OcrText, Usage, *ocrstore.CheckResult, string) error
}

// Options identifies the versioned checker and authorized spend.
type Options struct {
	Model, Effort            string
	LimitDocs                int
	MaxUSD                   float64
	Batch, DryRun, Reconcile bool
	Abandon                  string
}

// CheckerID binds the model, effort, instruction and schema versions.
func (o Options) CheckerID() string { return o.Model + "/" + o.Effort + "/ocr-check-2" }

// Validate rejects unsupported checker settings and recovery combinations.
func (o Options) Validate() error {
	if _, err := modelPrice(o.Model, o.Batch); err != nil {
		return err
	}
	switch o.Effort {
	case "low", "medium", "high", "xhigh", "max":
	default:
		return fmt.Errorf("unsupported OCR checker effort")
	}
	if o.LimitDocs < 0 || o.MaxUSD <= 0 || math.IsNaN(o.MaxUSD) || math.IsInf(o.MaxUSD, 0) || o.MaxUSD > 1e9 {
		return fmt.Errorf("a positive finite --max-usd is required; --limit-docs must be nonnegative")
	}
	if err := validateRecoveryOptions(o); err != nil {
		return err
	}
	return nil
}

// Summary counts completed, failed and submitted work and its estimated cost.
type Summary struct {
	Complete, Failed, Submitted, Estimated int
	Pending, Attached, Abandoned           int
	CostMicroUSD, EstimateMicroUSD         int64
	Documents                              []DocumentSummary
}

// Runner applies checks one request at a time.
type Runner struct {
	Client     Client
	Renderer   Renderer
	Repository Repository
	CountPages func(context.Context, []byte) (int, error)
	Now        func() time.Time
	batchBytes int
}

type prepared struct {
	doc         Document
	request     Request
	pages       int
	worst       int64
	inputTokens int64
	rendering   *RenderStats
}

// Run collects earlier submissions before selecting any new paid work.
func (r *Runner) Run(ctx context.Context, docs []Document, o Options) (Summary, error) {
	var summary Summary
	if err := o.Validate(); err != nil {
		return summary, err
	}
	if r.Now == nil {
		r.Now = time.Now
	}
	if r.CountPages == nil {
		r.CountPages = countPages
	}
	p, _ := modelPrice(o.Model, o.Batch)
	spent, err := r.Repository.Spent(ctx, o.CheckerID())
	if err != nil {
		return summary, fmt.Errorf("read OCR spend: %w", err)
	}
	summary.CostMicroUSD = spent
	skip := map[int64]bool{}
	if !o.DryRun {
		pending, resumeErr := r.resume(ctx, docs, o, &summary, skip)
		if resumeErr != nil || pending || o.Reconcile || o.Abandon != "" {
			return summary, resumeErr
		}
	}
	ceiling := int64(math.Floor(o.MaxUSD * 1e6))
	reserved := summary.CostMicroUSD
	var group []prepared
	var size int
	flush := func() error {
		if len(group) == 0 {
			return nil
		}
		submitErr := r.submit(ctx, group, o, &summary)
		group = nil
		size = 0
		return submitErr
	}
	selected := 0
	for _, doc := range docs {
		if ctx.Err() != nil {
			return summary, safeError(ctx.Err())
		}
		if skip[doc.Entry.ID] {
			continue
		}
		skip[doc.Entry.ID] = true
		complete, checkErr := r.Repository.Completed(ctx, doc.Entry.ID, o.CheckerID())
		if checkErr != nil {
			return summary, fmt.Errorf("read OCR check state: %w", checkErr)
		}
		if complete {
			continue
		}
		if o.LimitDocs > 0 && selected >= o.LimitDocs {
			break
		}
		selected++
		ready, reason := r.prepare(ctx, doc, o)
		if reason != "" {
			if !o.DryRun {
				if err = r.failPrepared(ctx, ready, o, reason); err != nil {
					return summary, err
				}
			}
			summary.Failed++
			summary.record(doc, ready.pages, Usage{}, reason)
			continue
		}
		tokens, countErr := r.Client.CountTokens(ctx, ready.request)
		if countErr != nil || tokens < 0 || tokens > 1_000_000_000 {
			u := errorUsage(countErr)
			u.Rendering = ready.rendering
			u.PDFPages = ready.pages
			summary.record(doc, ready.pages, u, errorReason(countErr))
			if !o.DryRun {
				if err = r.failAttempt(ctx, doc, o, errorReason(countErr), u); err != nil {
					return summary, err
				}
				summary.Failed++
			}
			if o.DryRun {
				return summary, safeError(countErr)
			}
			continue
		}
		ready.inputTokens = tokens
		ready.worst = p.cost(Usage{Input: tokens, Output: maxOutputTokens})
		if ready.worst > ceiling-reserved {
			if err = flush(); err != nil {
				return summary, err
			}
			return summary, ErrBudget
		}
		if o.DryRun {
			summary.record(doc, ready.pages, Usage{Input: tokens, CostMicroUSD: ready.worst}, "estimated")
			summary.Estimated++
			summary.EstimateMicroUSD += ready.worst
			reserved += ready.worst
			continue
		}
		if !o.Batch {
			if err = r.direct(ctx, ready, o, &summary); err != nil {
				return summary, err
			}
			reserved = summary.CostMicroUSD
			continue
		}
		n := requestSize(ready.request)
		limit := 200_000_000
		if r.batchBytes > 0 {
			limit = r.batchBytes
		}
		if n+len(`{"requests":[]}`) > limit {
			if err = r.failPrepared(ctx, ready, o, "over_check_limit"); err != nil {
				return summary, err
			}
			summary.Failed++
			summary.record(doc, ready.pages, Usage{}, "over_check_limit")
			continue
		}
		if len(group) >= 100_000 || size+n+len(`{"requests":[]}`) > limit {
			if err = flush(); err != nil {
				return summary, err
			}
		}
		group = append(group, ready)
		size += n
		reserved += ready.worst
	}
	return summary, flush()
}
