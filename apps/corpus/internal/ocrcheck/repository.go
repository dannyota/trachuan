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
		total += u.CostMicroUSD
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

// Begin records every batch item atomically before the provider is called.
func (d *Database) Begin(ctx context.Context, entries []store.OcrText, checker, mode, submission string, _ time.Time) ([]Attempt, error) {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin OCR intent: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := store.New(tx)
	attempts := make([]Attempt, 0, len(entries))
	for _, entry := range entries {
		id, insertErr := q.InsertOCRCheckAttempt(ctx, store.InsertOCRCheckAttemptParams{OcrTextID: entry.ID, CheckerID: checker, Mode: mode, SubmissionID: submission})
		if insertErr != nil {
			return nil, fmt.Errorf("insert OCR intent: %w", insertErr)
		}
		attempts = append(attempts, Attempt{ID: id, EntryID: entry.ID, Checker: checker, Mode: mode, SubmissionID: submission, State: "preparing"})
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

// Abandon releases an uncertain submission after exhaustive reconciliation.
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
	if result != nil {
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
