package ocrcheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func requestSize(r Request) int {
	// Reserve the longest int64 row id and the 128-bit submission prefix.
	r.CustomID = strings.Repeat("a", 32) + "-9223372036854775807"
	b, err := json.Marshal(batchRequest(r))
	if err != nil {
		return 200_000_001
	}
	return len(b) + 1
}
func (r *Runner) submit(ctx context.Context, group []prepared, o Options, s *Summary) error {
	docs := make([]Document, len(group))
	for i, p := range group {
		docs[i] = p.doc
	}
	attempts, err := r.begin(ctx, group, o)
	if err != nil {
		return err
	}
	for _, a := range attempts {
		s.CostMicroUSD += a.Usage.charge()
	}
	requests := make([]Request, len(group))
	for i, p := range group {
		requests[i] = p.request
		requests[i].CustomID = customID(attempts[i])
	}
	batch, err := r.Client.CreateBatch(ctx, requests)
	if err != nil {
		var apiErr *APIError
		if errors.As(safeError(err), &apiErr) && apiErr.Status >= 400 && apiErr.Status < 500 && apiErr.Status != 408 && apiErr.RequestID != "" {
			for i, a := range attempts {
				u := Usage{Final: true, APIStatus: apiErr.Status, RequestID: apiErr.RequestID}.withReservation(a.Usage)
				if finishErr := r.Repository.Finish(ctx, a, docs[i].Entry, u, nil, "api_error"); finishErr != nil {
					return fmt.Errorf("record rejected OCR batch: %w", finishErr)
				}
				s.CostMicroUSD -= a.Usage.charge()
				s.Failed++
			}
			return apiErr
		}
		// The provider may have accepted a request whose response was lost.
		if markErr := r.Repository.Uncertain(ctx, attempts[0].SubmissionID); markErr != nil {
			return fmt.Errorf("record uncertain OCR batch: %w", markErr)
		}
		return safeError(err)
	}
	if batch.ID == "" {
		return fmt.Errorf("provider returned no OCR batch identity; submission remains preparing")
	}
	if err = r.Repository.Submitted(ctx, attempts[0].SubmissionID, batch.ID); err != nil {
		return fmt.Errorf("attach OCR batch identity: %w", err)
	}
	s.Submitted += len(group)
	return nil
}
func customID(a Attempt) string { return a.SubmissionID + "-" + strconv.FormatInt(a.EntryID, 10) }

func (r *Runner) resume(ctx context.Context, docs []Document, o Options, s *Summary, skip map[int64]bool) (bool, error) {
	attempts, err := r.Repository.Open(ctx, o.CheckerID())
	if err != nil {
		return false, fmt.Errorf("read open OCR attempts: %w", err)
	}
	stale := map[string]bool{}
	for i, a := range attempts {
		skip[a.EntryID] = true
		if a.State == "preparing" && r.Now().Sub(a.CreatedAt) > 10*time.Minute {
			if !stale[a.SubmissionID] {
				if err = r.Repository.Uncertain(ctx, a.SubmissionID); err != nil {
					return false, fmt.Errorf("mark stale OCR submission: %w", err)
				}
				stale[a.SubmissionID] = true
			}
			attempts[i].State = "uncertain"
		}
	}
	if o.Abandon != "" {
		return false, r.abandon(ctx, attempts, o.Abandon, s)
	}
	if o.Reconcile {
		return false, r.reconcile(ctx, attempts, s)
	}
	for _, a := range attempts {
		if a.State == "uncertain" {
			return true, fmt.Errorf("uncertain OCR submission %s blocks new work; use --reconcile", a.SubmissionID)
		}
	}
	byID := map[int64]Document{}
	for _, d := range docs {
		byID[d.Entry.ID] = d
	}
	batches := map[string][]Attempt{}
	pending := false
	for _, a := range attempts {
		if a.State == "preparing" {
			pending = true
			s.Pending++
			continue
		}
		if a.Mode != "batch" || a.BatchID == "" {
			return true, fmt.Errorf("invalid submitted OCR attempt")
		}
		batches[a.BatchID] = append(batches[a.BatchID], a)
	}
	for id, group := range batches {
		batch, getErr := r.Client.GetBatch(ctx, id)
		if getErr != nil {
			return true, safeError(getErr)
		}
		if !batch.Ended {
			pending = true
			s.Pending += len(group)
			continue
		}
		results, resultErr := r.Client.BatchResults(ctx, id)
		if resultErr != nil {
			return true, safeError(resultErr)
		}
		matched := map[string]BatchResult{}
		for _, v := range results {
			if _, exists := matched[v.CustomID]; exists {
				return true, fmt.Errorf("duplicate OCR batch result identity")
			}
			matched[v.CustomID] = v
		}
		// Validate coverage first. A truncated download must not release any attempt.
		for _, a := range group {
			if _, ok := matched[customID(a)]; !ok {
				return true, fmt.Errorf("OCR batch results omit an open attempt")
			}
		}
		for _, a := range group {
			doc, ok := byID[a.EntryID]
			if !ok {
				return true, fmt.Errorf("OCR batch entry %d is absent from this run", a.EntryID)
			}
			result := matched[customID(a)]
			if result.Type != "succeeded" {
				reason := "api_error"
				switch result.Type {
				case "expired":
					reason = "expired"
				case "canceled":
					reason = "canceled"
				}
				if err = r.Repository.Finish(ctx, a, doc.Entry, Usage{Final: true, RequestID: result.Response.Usage.RequestID}.withReservation(a.Usage), nil, reason); err != nil {
					return true, fmt.Errorf("finish OCR batch failure: %w", err)
				}
				s.CostMicroUSD -= a.Usage.charge()
				s.Failed++
				continue
			}
			if !result.Response.FinalUsage {
				return true, r.unknown(ctx, a, doc, result.Response, nil, o, s)
			}
			p, reason := r.prepare(ctx, doc, o)
			if reason != "" {
				price, _ := modelPrice(o.Model, true)
				u := result.Response.Usage.withReservation(a.Usage)
				u.Final = true
				u.CostMicroUSD = price.cost(u)
				if err = r.Repository.Finish(ctx, a, doc.Entry, u, nil, reason); err != nil {
					return true, fmt.Errorf("finish OCR batch validation: %w", err)
				}
				s.CostMicroUSD += u.charge() - a.Usage.charge()
				s.Failed++
				if priceErr := u.priceError(); priceErr != nil {
					return true, priceErr
				}
				continue
			}
			if err = r.finish(ctx, a, doc, p.pages, result.Response, nil, o, s); err != nil {
				return true, err
			}
		}
	}
	return pending, nil
}
