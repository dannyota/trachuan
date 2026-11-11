package ocrcheck

import (
	"context"
	"crypto/rand"
	"errors"
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
}

// Repository persists attempts and completed checks atomically.
type Repository interface {
	Open(context.Context, string) ([]Attempt, error)
	Spent(context.Context, string) (int64, error)
	Completed(context.Context, int64, string) (bool, error)
	Begin(context.Context, []store.OcrText, string, string, string, time.Time) ([]Attempt, error)
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
}

// CheckerID binds the model, effort, instruction and schema versions.
func (o Options) CheckerID() string { return o.Model + "/" + o.Effort + "/ocr-check-1" }
func (o Options) validate() error {
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
	if o.Reconcile && o.DryRun {
		return fmt.Errorf("--reconcile and --dry-run cannot be combined")
	}
	return nil
}

// Summary counts completed, failed and submitted work and its estimated cost.
type Summary struct {
	Complete, Failed, Submitted, Estimated int
	Pending, Attached, Abandoned           int
	CostMicroUSD, EstimateMicroUSD         int64
}

// Runner applies checks serially, keeping direct concurrency below three.
type Runner struct {
	Client       Client
	Repository   Repository
	CountPages   func(context.Context, []byte) (int, error)
	Now          func() time.Time
	batchBytes   int
	requestBytes func(Request) int
}

type prepared struct {
	doc     Document
	request Request
	pages   int
	worst   int64
}

// Run collects earlier submissions before selecting any new paid work.
func (r *Runner) Run(ctx context.Context, docs []Document, o Options) (Summary, error) {
	var summary Summary
	if err := o.validate(); err != nil {
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
		if resumeErr != nil || pending || o.Reconcile {
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
				if err = r.failLocal(ctx, doc, o, reason); err != nil {
					return summary, err
				}
			}
			summary.Failed++
			continue
		}
		tokens, countErr := r.Client.CountTokens(ctx, ready.request)
		if countErr != nil || tokens < 0 || tokens > 1_000_000_000 {
			if !o.DryRun {
				if err = r.failAttempt(ctx, doc, o, errorReason(countErr), errorUsage(countErr)); err != nil {
					return summary, err
				}
				summary.Failed++
			}
			if o.DryRun {
				return summary, safeError(countErr)
			}
			continue
		}
		ready.worst = p.cost(Usage{Input: tokens, Output: maxOutputTokens})
		if ready.worst > ceiling-reserved {
			if err = flush(); err != nil {
				return summary, err
			}
			return summary, ErrBudget
		}
		if o.DryRun {
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
		if r.requestBytes != nil {
			n = r.requestBytes(ready.request)
		}
		limit := 200_000_000
		if r.batchBytes > 0 {
			limit = r.batchBytes
		}
		if n+len(`{"requests":[]}`) > limit {
			if err = r.failLocal(ctx, doc, o, "over_check_limit"); err != nil {
				return summary, err
			}
			summary.Failed++
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

func newSubmission() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("create OCR submission identity: %w", err)
	}
	return fmt.Sprintf("%x", id), nil
}
func (r *Runner) begin(ctx context.Context, docs []Document, o Options) ([]Attempt, error) {
	id, err := newSubmission()
	if err != nil {
		return nil, err
	}
	entries := make([]store.OcrText, len(docs))
	for i, d := range docs {
		entries[i] = d.Entry
	}
	mode := "direct"
	if o.Batch {
		mode = "batch"
	}
	a, err := r.Repository.Begin(ctx, entries, o.CheckerID(), mode, id, r.Now())
	if err != nil {
		return nil, fmt.Errorf("record OCR submission intent: %w", err)
	}
	return a, nil
}
func (r *Runner) failLocal(ctx context.Context, doc Document, o Options, reason string) error {
	return r.failAttempt(ctx, doc, o, reason, Usage{})
}
func (r *Runner) failAttempt(ctx context.Context, doc Document, o Options, reason string, usage Usage) error {
	a, err := r.begin(ctx, []Document{doc}, o)
	if err != nil {
		return err
	}
	return r.Repository.Finish(ctx, a[0], doc.Entry, usage, nil, reason)
}
func (r *Runner) direct(ctx context.Context, p prepared, o Options, s *Summary) error {
	a, err := r.begin(ctx, []Document{p.doc}, o)
	if err != nil {
		return err
	}
	response, callErr := r.Client.Direct(ctx, p.request)
	return r.finish(ctx, a[0], p.doc, p.pages, response, callErr, o, s)
}
func (r *Runner) finish(ctx context.Context, a Attempt, doc Document, pages int, response Response, callErr error, o Options, s *Summary) error {
	price, _ := modelPrice(o.Model, a.Mode == "batch")
	response.Usage.CostMicroUSD = price.cost(response.Usage)
	var result *ocrstore.CheckResult
	reason := ""
	if callErr != nil {
		reason = errorReason(callErr)
		var apiErr *APIError
		if errors.As(safeError(callErr), &apiErr) {
			response.Usage.APIStatus = apiErr.Status
			response.Usage.RequestID = apiErr.RequestID
		}
	} else {
		switch response.Stop {
		case "end_turn":
			parsed, err := ocrstore.DecodeAnswer([]byte(response.Text), pages)
			if err != nil {
				reason = "schema"
			} else if parsed.Pages != pages || (doc.Entry.Pages.Valid && parsed.Pages != int(doc.Entry.Pages.Int32)) {
				reason = "page_count_mismatch"
			} else {
				result = &parsed
			}
		case "refusal":
			reason = "refusal"
		case "max_tokens":
			reason = "max_tokens"
		case "model_context_window_exceeded":
			reason = "context_window"
		default:
			reason = "schema"
		}
	}
	if err := r.Repository.Finish(ctx, a, doc.Entry, response.Usage, result, reason); err != nil {
		return fmt.Errorf("finish OCR attempt: %w", err)
	}
	s.CostMicroUSD += response.Usage.CostMicroUSD
	if result == nil {
		s.Failed++
	} else {
		s.Complete++
	}
	return response.Usage.priceError()
}
func errorReason(err error) string {
	var e *APIError
	if errors.As(err, &e) && e.Reason == "timeout" {
		return "timeout"
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "timeout"
	}
	return "api_error"
}
func safeError(err error) error {
	if err == nil {
		return &APIError{Reason: "api_error"}
	}
	var e *APIError
	if errors.As(err, &e) {
		return e
	}
	return &APIError{Reason: errorReason(err)}
}

func errorUsage(err error) Usage {
	var apiErr *APIError
	if errors.As(safeError(err), &apiErr) {
		return Usage{APIStatus: apiErr.Status, RequestID: apiErr.RequestID}
	}
	return Usage{}
}
