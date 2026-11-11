package ocrcheck

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

// Intent keeps the counted worst-case charge with the entry before sending.
type Intent struct {
	Entry store.OcrText
	Usage Usage
}

func newSubmission() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("create OCR submission identity: %w", err)
	}
	return fmt.Sprintf("%x", id), nil
}
func (r *Runner) begin(ctx context.Context, group []prepared, o Options) ([]Attempt, error) {
	id, err := newSubmission()
	if err != nil {
		return nil, err
	}
	intents := make([]Intent, len(group))
	for i, p := range group {
		u := Usage{Final: p.worst == 0, CountedInput: p.inputTokens, MaxOutput: maxOutputTokens, ReservationMicroUSD: p.worst, Rendering: p.rendering, PDFPages: p.pages}
		intents[i] = Intent{Entry: p.doc.Entry, Usage: u}
	}
	mode := "direct"
	if o.Batch {
		mode = "batch"
	}
	attempts, err := r.Repository.Begin(ctx, intents, o.CheckerID(), mode, id, r.Now())
	if err != nil {
		return nil, fmt.Errorf("record OCR submission intent: %w", err)
	}
	return attempts, nil
}
func (r *Runner) failPrepared(ctx context.Context, p prepared, o Options, reason string) error {
	return r.failAttempt(ctx, p.doc, o, reason, Usage{Rendering: p.rendering, PDFPages: p.pages})
}
func (r *Runner) failAttempt(ctx context.Context, doc Document, o Options, reason string, u Usage) error {
	attempts, err := r.begin(ctx, []prepared{{doc: doc}}, o)
	if err != nil {
		return err
	}
	u.Final = true
	return r.Repository.Finish(ctx, attempts[0], doc.Entry, u, nil, reason)
}
func (r *Runner) direct(ctx context.Context, p prepared, o Options, s *Summary) error {
	attempts, err := r.begin(ctx, []prepared{p}, o)
	if err != nil {
		return err
	}
	a := attempts[0]
	s.CostMicroUSD += a.Usage.charge()
	response, callErr := r.Client.Direct(ctx, p.request)
	if !response.FinalUsage {
		if preGenerationFailure(response, callErr) {
			response.Usage = errorUsage(callErr)
		} else {
			return r.unknown(ctx, a, p.doc, response, callErr, o, s)
		}
	}
	return r.finish(ctx, a, p, response, callErr, o, s)
}
func preGenerationFailure(response Response, err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status < 500 && apiErr.Status != 408 && apiErr.RequestID != "" && response.Usage.Input == 0 && response.Usage.Output == 0 && response.Usage.CacheRead == 0 && response.Usage.CacheCreation == 0 && response.Text == "" && response.Stop == ""
}
func (r *Runner) unknown(ctx context.Context, a Attempt, doc Document, response Response, callErr error, o Options, s *Summary) error {
	p, _ := modelPrice(o.Model, a.Mode == "batch")
	u := response.Usage.withReservation(a.Usage)
	u.Final = false
	u.CostMicroUSD = p.cost(u)
	metadata := errorUsage(callErr)
	u.APIStatus = metadata.APIStatus
	u.RequestID = metadata.RequestID
	if err := r.Repository.Finish(ctx, a, doc.Entry, u, nil, errorReason(callErr)); err != nil {
		return fmt.Errorf("record uncertain OCR billing: %w", err)
	}
	s.CostMicroUSD += u.charge() - a.Usage.charge()
	s.Pending++
	s.record(doc, a.Usage.PDFPages, u, "uncertain:"+errorReason(callErr))
	return fmt.Errorf("uncertain OCR submission %s: final usage did not arrive; worst-case charge retained and further spending blocked", a.SubmissionID)
}
func (r *Runner) finish(ctx context.Context, a Attempt, ready prepared, response Response, callErr error, o Options, s *Summary) error {
	doc := ready.doc
	p, _ := modelPrice(o.Model, a.Mode == "batch")
	u := response.Usage.withReservation(a.Usage)
	u.Final = true
	u.CostMicroUSD = p.cost(u)
	var result *ocrstore.CheckResult
	reason := ""
	if callErr != nil {
		reason = errorReason(callErr)
		metadata := errorUsage(callErr)
		u.APIStatus = metadata.APIStatus
		u.RequestID = metadata.RequestID
	} else {
		switch response.Stop {
		case "end_turn":
			parsed, invalid := decodeAnswer([]byte(response.Text), ready.pages, ready.request.Text)
			if invalid != "" {
				reason = invalid
			} else if doc.Entry.Pages.Valid && parsed.Pages != int(doc.Entry.Pages.Int32) {
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
			reason = "schema:stop_reason"
		}
	}
	if err := r.Repository.Finish(ctx, a, doc.Entry, u, result, reason); err != nil {
		return fmt.Errorf("finish OCR attempt: %w", err)
	}
	s.CostMicroUSD += u.charge() - a.Usage.charge()
	outcome := reason
	if result != nil {
		outcome = result.Verdict
	}
	s.record(doc, ready.pages, u, outcome)
	if result == nil {
		s.Failed++
	} else {
		s.Complete++
	}
	return u.priceError()
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
	var e *APIError
	if errors.As(safeError(err), &e) {
		return Usage{APIStatus: e.Status, RequestID: e.RequestID}
	}
	return Usage{}
}
