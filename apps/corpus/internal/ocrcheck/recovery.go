package ocrcheck

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

func validateRecoveryOptions(o Options) error {
	if o.RetryFailed && (o.Reconcile || o.Abandon != "") {
		return fmt.Errorf("--retry-failed cannot be combined with --reconcile or --abandon")
	}
	if o.Reconcile && o.DryRun {
		return fmt.Errorf("--reconcile and --dry-run cannot be combined")
	}
	if o.Abandon != "" {
		decoded, err := hex.DecodeString(o.Abandon)
		if err != nil || len(decoded) != 16 || strings.ToLower(o.Abandon) != o.Abandon {
			return fmt.Errorf("--abandon requires a 32-character lowercase hex submission id")
		}
		if o.Reconcile || o.DryRun || o.Batch || o.LimitDocs != 0 {
			return fmt.Errorf("--abandon cannot be combined with --reconcile, --dry-run, --batch or --limit-docs")
		}
	}
	return nil
}
func (r *Runner) abandon(ctx context.Context, attempts []Attempt, id string, s *Summary) error {
	var group []Attempt
	for _, a := range attempts {
		if a.SubmissionID != id {
			continue
		}
		if a.State != "uncertain" {
			return fmt.Errorf("only uncertain OCR submissions can be abandoned")
		}
		if r.Now().Before(a.CreatedAt.Add(24 * time.Hour)) {
			return fmt.Errorf("cannot abandon OCR submission within 24 hours of its attempt")
		}
		group = append(group, a)
	}
	if len(group) == 0 {
		return fmt.Errorf("uncertain OCR submission not found for this checker")
	}
	if err := r.Repository.Abandon(ctx, id); err != nil {
		return fmt.Errorf("record operator abandonment: %w", err)
	}
	s.Abandoned += len(group)
	return nil
}
func (r *Runner) reconcile(ctx context.Context, attempts []Attempt, s *Summary) error {
	groups := map[string][]Attempt{}
	for _, a := range attempts {
		if a.State == "uncertain" {
			groups[a.SubmissionID] = append(groups[a.SubmissionID], a)
		}
	}
	for submission, group := range groups {
		first := group[0]
		if first.Mode != "batch" {
			return fmt.Errorf("uncertain direct OCR submission %s needs operator resolution; provider batch reconciliation cannot prove its spend", submission)
		}
		batches, err := r.Client.ListBatches(ctx, first.CreatedAt.Add(-time.Minute), first.CreatedAt.Add(10*time.Minute))
		if err != nil {
			return safeError(err)
		}
		found := ""
		for _, batch := range batches {
			if !batch.Ended {
				continue
			}
			results, resultErr := r.Client.BatchResults(ctx, batch.ID)
			if resultErr != nil {
				return safeError(resultErr)
			}
			for _, v := range results {
				if !strings.HasPrefix(v.CustomID, submission+"-") {
					continue
				}
				if found != "" && found != batch.ID {
					return fmt.Errorf("multiple provider batches match OCR submission %s", submission)
				}
				found = batch.ID
			}
		}
		if found == "" {
			s.Pending += len(group)
			continue
		}
		if err = r.Repository.Submitted(ctx, submission, found); err != nil {
			return fmt.Errorf("reconcile OCR batch: %w", err)
		}
		s.Attached += len(group)
	}
	return nil
}
