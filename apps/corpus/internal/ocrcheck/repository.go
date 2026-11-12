package ocrcheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Database persists check state in the workshop database.
type Database struct {
	pool *pgxpool.Pool
	root string
}

// NewDatabase constructs the transactional check repository.
func NewDatabase(pool *pgxpool.Pool, root string) *Database { return &Database{pool: pool, root: root} }

// Open returns attempts that must be resolved before new submissions.
func (d *Database) Open(ctx context.Context, checker string) ([]Attempt, error) {
	rows, err := store.New(d.pool).ListOpenOCRCheckAttempts(ctx, checker)
	if err != nil {
		return nil, fmt.Errorf("list open OCR attempts: %w", err)
	}
	result := make([]Attempt, len(rows))
	for i, v := range rows {
		a := Attempt{ID: v.ID, EntryID: v.OcrTextID, Checker: v.CheckerID, Mode: v.Mode, SubmissionID: v.SubmissionID, State: v.State, Reason: v.Reason, CreatedAt: v.CreatedAt}
		if v.BatchID != nil {
			a.BatchID = *v.BatchID
		}
		if err = json.Unmarshal(v.Usage, &a.Usage); err != nil {
			return nil, fmt.Errorf("invalid OCR intent usage")
		}
		if !a.Usage.Final && a.Usage.ReservationMicroUSD <= 0 {
			return nil, fmt.Errorf("open OCR attempt has no durable reservation; operator resolution required")
		}
		result[i] = a
	}
	return result, nil
}

// Spent sums recorded charges from both successful and failed attempts.
func (d *Database) Spent(ctx context.Context, checker string) (int64, error) {
	rows, err := store.New(d.pool).SumOCRCheckUsage(ctx, checker)
	if err != nil {
		return 0, fmt.Errorf("read OCR usage: %w", err)
	}
	var total int64
	for _, data := range rows {
		var u Usage
		if err = json.Unmarshal(data, &u); err != nil {
			return 0, fmt.Errorf("invalid stored OCR usage")
		}
		if err = u.priceError(); err != nil {
			return 0, err
		}
		total += u.charge()
	}
	// The existing aggregate excludes submitted attempts; include their holds.
	open, err := d.Open(ctx, checker)
	if err != nil {
		return 0, err
	}
	for _, a := range open {
		if a.State == "submitted" {
			total += a.Usage.charge()
		}
	}
	return total, nil
}

// Completed reports whether immutable evidence already exists.
func (d *Database) Completed(ctx context.Context, id int64, checker string) (bool, error) {
	_, err := store.New(d.pool).GetOCRCheck(ctx, store.GetOCRCheckParams{OcrTextID: id, CheckerID: checker})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read completed OCR check: %w", err)
	}
	return true, nil
}

// RetryState counts billed failures and identifies a latest unbilled failure.
func (d *Database) RetryState(ctx context.Context, id int64, checker string) (int, bool, error) {
	rows, err := store.New(d.pool).ListOCRCheckAttemptHistory(ctx, store.ListOCRCheckAttemptHistoryParams{OcrTextID: id, CheckerID: checker})
	if err != nil {
		return 0, false, fmt.Errorf("read OCR attempt history: %w", err)
	}
	count := 0
	latestUnbilled := false
	for i, row := range rows {
		if row.State != "failed" {
			continue
		}
		var usage Usage
		if err = json.Unmarshal(row.Usage, &usage); err != nil {
			return 0, false, fmt.Errorf("invalid stored OCR usage: %w", err)
		}
		billed := usage.CostMicroUSD > 0 || usage.Input > 0
		if billed {
			count++
		} else if i == 0 {
			latestUnbilled = true
		}
	}
	return count, latestUnbilled, nil
}

// Begin records every batch item atomically before the provider is called.
func (d *Database) Begin(ctx context.Context, intents []Intent, checker, mode, submission string, now time.Time) ([]Attempt, error) {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin OCR intent: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := store.New(tx)
	attempts := make([]Attempt, 0, len(intents))
	for _, intent := range intents {
		entry := intent.Entry
		id, insertErr := q.InsertOCRCheckAttempt(ctx, store.InsertOCRCheckAttemptParams{OcrTextID: entry.ID, CheckerID: checker, Mode: mode, SubmissionID: submission})
		if insertErr != nil {
			return nil, fmt.Errorf("insert OCR intent: %w", insertErr)
		}
		usage, encodeErr := json.Marshal(intent.Usage)
		if encodeErr != nil {
			return nil, fmt.Errorf("encode OCR reservation: %w", encodeErr)
		}
		n, updateErr := q.FinishOCRCheckAttempt(ctx, store.FinishOCRCheckAttemptParams{ID: id, State: "preparing", Usage: usage})
		if updateErr = changed(n, updateErr); updateErr != nil {
			return nil, updateErr
		}
		attempts = append(attempts, Attempt{ID: id, EntryID: entry.ID, Checker: checker, Mode: mode, SubmissionID: submission, State: "preparing", CreatedAt: now, Usage: intent.Usage})
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit OCR intent: %w", err)
	}
	return attempts, nil
}

// Submitted attaches a known provider identity to a whole submission.
func (d *Database) Submitted(ctx context.Context, submission, batch string) error {
	var id *string
	if batch != "" {
		id = &batch
	}
	n, err := store.New(d.pool).MarkOCRCheckAttemptsSubmitted(ctx, store.MarkOCRCheckAttemptsSubmittedParams{SubmissionID: submission, BatchID: id})
	return changed(n, err)
}

// Uncertain blocks resubmission after an ambiguous provider response.
func (d *Database) Uncertain(ctx context.Context, submission string) error {
	n, err := store.New(d.pool).MarkOCRCheckAttemptsUncertain(ctx, store.MarkOCRCheckAttemptsUncertainParams{SubmissionID: &submission, Reason: "api_error"})
	return changed(n, err)
}

// Abandon records the operator decision without changing the usage reservation.
func (d *Database) Abandon(ctx context.Context, submission string) error {
	n, err := store.New(d.pool).AbandonOCRCheckAttempts(ctx, submission)
	return changed(n, err)
}

// Finish commits evidence and its billed attempt together, or neither.
func (d *Database) Finish(ctx context.Context, a Attempt, entry store.OcrText, u Usage, result *ocrstore.CheckResult, reason string) error {
	// A canceled provider request still needs its usage and terminal state saved.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin OCR result: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	state := "failed"
	if !u.Final && u.ReservationMicroUSD > 0 {
		state = "uncertain"
	}
	if result != nil {
		if !u.Final {
			return fmt.Errorf("completed check requires final usage")
		}
		data, encodeErr := json.Marshal(result)
		if encodeErr != nil {
			return fmt.Errorf("encode OCR result: %w", encodeErr)
		}
		if _, err = ocrstore.Open(d.root, tx).PutCheck(ctx, entry, a.Checker, data, int32(len(result.Items)), true); err != nil {
			return err
		}
		state = "complete"
	}
	data, err := json.Marshal(u)
	if err != nil {
		return fmt.Errorf("encode OCR usage: %w", err)
	}
	n, err := store.New(tx).FinishOCRCheckAttempt(ctx, store.FinishOCRCheckAttemptParams{ID: a.ID, State: state, Reason: reason, Usage: data})
	if err = changed(n, err); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit OCR result: %w", err)
	}
	return nil
}
func changed(n int64, err error) error {
	if err != nil {
		return fmt.Errorf("update OCR attempt: %w", err)
	}
	if n < 1 {
		return fmt.Errorf("OCR attempt state changed concurrently")
	}
	return nil
}
