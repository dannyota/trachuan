package membership

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/store"
)

// Phase identifies a frozen membership stage.
type Phase string

// Membership phases in publication order.
const (
	Seeds    Phase = "seeds"
	Expanded Phase = "expanded"
	Final    Phase = "final"
)

// PhaseSnapshot contains a verified snapshot and its membership views.
type PhaseSnapshot struct {
	Entries          []Entry
	Hash             string
	Active           []Entry
	RetainedRemovals []Entry
}

// PublishPhase freezes canonical membership using a transaction-bound Queries.
func PublishPhase(ctx context.Context, q *store.Queries, runID int64, phase Phase, entries []Entry) error {
	data, hash, err := CanonicalSnapshot(entries)
	if err != nil {
		return err
	}
	if phase != Seeds && phase != Expanded && phase != Final {
		return fmt.Errorf("unknown membership phase %q", phase)
	}
	if scopeErr := q.EnsureRunScope(ctx, runID); scopeErr != nil {
		return fmt.Errorf("create scope: %w", scopeErr)
	}
	var n int64
	switch phase {
	case Seeds:
		n, err = q.PublishSeeds(ctx, store.PublishSeedsParams{RunID: runID, SeedsSnapshot: data, SeedsHash: &hash})
	case Expanded:
		n, err = q.PublishExpanded(ctx, store.PublishExpandedParams{RunID: runID, ExpandedSnapshot: data, ExpandedHash: &hash})
	case Final:
		n, err = q.PublishFinal(ctx, store.PublishFinalParams{RunID: runID, FinalSnapshot: data, FinalHash: &hash})
	}
	if err != nil {
		return fmt.Errorf("publish %s snapshot: %w", phase, err)
	}
	if n != 1 {
		return fmt.Errorf("membership phase %s is already frozen with different bytes", phase)
	}
	return nil
}

// ReadPhase reads and verifies a frozen snapshot using a transaction-bound Queries.
func ReadPhase(ctx context.Context, q *store.Queries, runID int64, phase Phase) (PhaseSnapshot, error) {
	row, err := q.GetRunScope(ctx, runID)
	if err != nil {
		return PhaseSnapshot{}, fmt.Errorf("read scope: %w", err)
	}
	var data []byte
	var hash *string
	switch phase {
	case Seeds:
		data, hash = row.SeedsSnapshot, row.SeedsHash
	case Expanded:
		data, hash = row.ExpandedSnapshot, row.ExpandedHash
	case Final:
		data, hash = row.FinalSnapshot, row.FinalHash
	default:
		return PhaseSnapshot{}, fmt.Errorf("unknown membership phase %q", phase)
	}
	if data == nil || hash == nil {
		return PhaseSnapshot{}, fmt.Errorf("membership phase %s is not frozen", phase)
	}
	var s snapshot
	if decodeErr := json.Unmarshal(data, &s); decodeErr != nil {
		return PhaseSnapshot{}, fmt.Errorf("decode snapshot: %w", decodeErr)
	}
	if s.Version != 1 {
		return PhaseSnapshot{}, fmt.Errorf("unknown membership snapshot version %d", s.Version)
	}
	canonical, actual, err := CanonicalSnapshot(s.Entries)
	if err != nil {
		return PhaseSnapshot{}, err
	}
	if actual != *hash || !bytes.Equal(data, canonical) {
		return PhaseSnapshot{}, fmt.Errorf("membership snapshot %s failed verification", phase)
	}
	result := PhaseSnapshot{Entries: s.Entries, Hash: *hash, Active: []Entry{}, RetainedRemovals: []Entry{}}
	for _, entry := range s.Entries {
		switch entry.Disposition {
		case "active":
			result.Active = append(result.Active, entry)
		case "withdrawn", "scope_removed":
			result.RetainedRemovals = append(result.RetainedRemovals, entry)
		}
	}
	return result, nil
}

// ClearPhase removes one phase reference so an authorized retry can republish it.
func ClearPhase(ctx context.Context, q *store.Queries, runID int64, phase Phase) error {
	var err error
	switch phase {
	case Seeds:
		err = q.ClearSeeds(ctx, runID)
	case Expanded:
		err = q.ClearExpanded(ctx, runID)
	case Final:
		err = q.ClearFinal(ctx, runID)
	default:
		return fmt.Errorf("unknown membership phase %q", phase)
	}
	if err != nil {
		return fmt.Errorf("clear %s snapshot: %w", phase, err)
	}
	return nil
}
