// Package sourcegate controls request admission for each run and source.
package sourcegate

import (
	"context"
	"errors"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

// ErrSourcePaused indicates that requests to the source are suspended.
var ErrSourcePaused = errors.New("source_paused")

// Gate controls admission and explicit changes to a source's pause state.
type Gate interface {
	Admit(context.Context, int64, string) error
	Pause(context.Context, int64, string, string, string) error
	Clear(context.Context, int64, string, string) error
}

// Beginner opens transactions for a database gate.
type Beginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

// Database is a gate backed by the workshop database.
type Database struct{ db Beginner }

// New constructs a database-backed gate.
func New(db Beginner) *Database { return &Database{db: db} }

// Admit checks the durable source state before a request.
func (g *Database) Admit(ctx context.Context, runID int64, source string) error {
	tx, err := g.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin source admission: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := store.New(tx)
	if _, err = q.GetRunGenerationForShare(ctx, runID); err != nil {
		return fmt.Errorf("lock run for admission: %w", err)
	}
	if err = q.EnsureRunSource(ctx, store.EnsureRunSourceParams{RunID: runID, Source: source}); err != nil {
		return fmt.Errorf("ensure source for admission: %w", err)
	}
	row, err := q.GetRunSourceForShare(ctx, store.GetRunSourceForShareParams{RunID: runID, Source: source})
	if err != nil {
		return fmt.Errorf("lock source for admission: %w", err)
	}
	if row.State != "active" {
		return ErrSourcePaused
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit source admission: %w", err)
	}
	return nil
}

// Pause records the first pause and its evidence atomically.
func (g *Database) Pause(ctx context.Context, runID int64, source, reason, evidence string) error {
	return g.change(ctx, runID, source, reason, evidence, false)
}

// Clear explicitly clears a pause and records the owner's note.
func (g *Database) Clear(ctx context.Context, runID int64, source, note string) error {
	return g.change(ctx, runID, source, note, "", true)
}

func (g *Database) change(ctx context.Context, runID int64, source, detail, evidence string, clear bool) error {
	tx, err := g.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin source state change: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := store.New(tx)
	// The run lock also serializes pauses with stage close and item commits.
	if _, err = q.GetControlRunForUpdate(ctx, runID); err != nil {
		return fmt.Errorf("lock run for source change: %w", err)
	}
	if err = q.EnsureRunSource(ctx, store.EnsureRunSourceParams{RunID: runID, Source: source}); err != nil {
		return fmt.Errorf("ensure source for state change: %w", err)
	}
	if _, err = q.GetRunSourceForUpdate(ctx, store.GetRunSourceForUpdateParams{RunID: runID, Source: source}); err != nil {
		return fmt.Errorf("lock source for state change: %w", err)
	}
	event := store.InsertRunSourceEventParams{RunID: runID, Source: source}
	var count int64
	if clear {
		count, err = q.ClearRunSource(ctx, store.ClearRunSourceParams{RunID: runID, Source: source})
		event.Event, event.ClearNote = "clear", &detail
	} else {
		var ref *string
		if evidence != "" {
			ref = &evidence
		}
		count, err = q.PauseRunSource(ctx, store.PauseRunSourceParams{RunID: runID, Source: source, Reason: &detail, EvidenceRef: ref})
		event.Event, event.Reason, event.EvidenceRef = "pause", &detail, ref
	}
	if err != nil {
		return fmt.Errorf("change source state: %w", err)
	}
	if count > 0 {
		if err = q.InsertRunSourceEvent(ctx, event); err != nil {
			return fmt.Errorf("record source event: %w", err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit source state change: %w", err)
	}
	return nil
}
