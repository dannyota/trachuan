package discover

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

type predecessor struct {
	id      *int64
	phase   *string
	hash    *string
	entries []membership.Entry
}

func selectPredecessor(ctx context.Context, q *store.Queries, r stage.Run, rows []store.ListEarlierRunsRow) (predecessor, error) {
	for _, row := range rows {
		scope, err := q.GetRunScope(ctx, row.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return predecessor{}, fmt.Errorf("read earlier scope: %w", err)
		}
		if scope.FinalSnapshot == nil {
			continue
		}
		snap, err := membership.ReadPhase(ctx, q, row.ID, membership.Final)
		if err != nil {
			return predecessor{}, fmt.Errorf("read predecessor final: %w", err)
		}
		phase := "final"
		return predecessor{id: &row.ID, phase: &phase, hash: &snap.Hash, entries: snap.Entries}, nil
	}
	for _, row := range rows {
		var snapshot stage.Snapshot
		if err := json.Unmarshal(row.Snapshot, &snapshot); err != nil {
			return predecessor{}, fmt.Errorf("decode earlier snapshot: %w", err)
		}
		list, err := source.Parse(snapshot.SourceList)
		if err != nil {
			return predecessor{}, fmt.Errorf("parse earlier sources: %w", err)
		}
		if list.VBPL.SelectionVersion != 0 {
			continue
		}
		entries, err := legacyMembership(ctx, q, row.ID, r.Country)
		if err != nil {
			return predecessor{}, err
		}
		_, hash, err := membership.CanonicalSnapshot(entries)
		if err != nil {
			return predecessor{}, fmt.Errorf("hash legacy predecessor: %w", err)
		}
		phase := "legacy"
		return predecessor{id: &row.ID, phase: &phase, hash: &hash, entries: entries}, nil
	}
	return predecessor{}, nil
}

func discoveryPredecessor(ctx context.Context, q *store.Queries, r stage.Run, rows []store.ListEarlierRunsRow) (predecessor, error) {
	if err := q.EnsureRunScope(ctx, r.ID); err != nil {
		return predecessor{}, fmt.Errorf("create scope: %w", err)
	}
	scope, err := q.GetRunScope(ctx, r.ID)
	if err != nil {
		return predecessor{}, fmt.Errorf("read scope: %w", err)
	}
	if scope.SeedsSnapshot != nil {
		return readPredecessor(ctx, q, r)
	}
	p, err := selectPredecessor(ctx, q, r, rows)
	if err != nil {
		return predecessor{}, err
	}
	n, err := q.SetMembershipPredecessor(ctx, store.SetMembershipPredecessorParams{RunID: r.ID, PredecessorRunID: p.id, PredecessorPhase: p.phase, PredecessorSnapshotHash: p.hash})
	if err != nil {
		return predecessor{}, fmt.Errorf("record predecessor: %w", err)
	}
	if n != 1 {
		return predecessor{}, fmt.Errorf("cannot record predecessor after seeds freeze")
	}
	return p, nil
}

func readPredecessor(ctx context.Context, q *store.Queries, r stage.Run) (predecessor, error) {
	scope, err := q.GetRunScope(ctx, r.ID)
	if err != nil {
		return predecessor{}, fmt.Errorf("read predecessor: %w", err)
	}
	p := predecessor{id: scope.PredecessorRunID, phase: scope.PredecessorPhase, hash: scope.PredecessorSnapshotHash}
	if p.id == nil && p.phase == nil && p.hash == nil {
		return p, nil
	}
	if p.id == nil || p.phase == nil || p.hash == nil {
		return predecessor{}, fmt.Errorf("incomplete predecessor record")
	}
	var actual string
	switch *p.phase {
	case "final":
		snap, e := membership.ReadPhase(ctx, q, *p.id, membership.Final)
		if e != nil {
			return predecessor{}, fmt.Errorf("read predecessor final: %w", e)
		}
		p.entries, actual = snap.Entries, snap.Hash
	case "legacy":
		p.entries, err = legacyMembership(ctx, q, *p.id, r.Country)
		if err != nil {
			return predecessor{}, err
		}
		_, actual, err = membership.CanonicalSnapshot(p.entries)
		if err != nil {
			return predecessor{}, fmt.Errorf("hash legacy predecessor: %w", err)
		}
	default:
		return predecessor{}, fmt.Errorf("unknown predecessor phase %q", *p.phase)
	}
	if actual != *p.hash {
		return predecessor{}, fmt.Errorf("predecessor snapshot hash mismatch")
	}
	return p, nil
}

// RecordedMembership reads the recorded predecessor and excludes accepted removals.
func RecordedMembership(ctx context.Context, q *store.Queries, r stage.Run) ([]membership.Entry, error) {
	p, err := readPredecessor(ctx, q, r)
	if err != nil {
		return nil, err
	}
	return unacceptedMembership(ctx, q, p)
}

func unacceptedMembership(ctx context.Context, q *store.Queries, p predecessor) ([]membership.Entry, error) {
	if p.id == nil {
		return p.entries, nil
	}
	gaps, err := q.ListAcceptedRemovalGaps(ctx, *p.id)
	if err != nil {
		return nil, fmt.Errorf("list accepted predecessor removals: %w", err)
	}
	accepted := map[string]map[string]bool{}
	for _, gap := range gaps {
		if accepted[gap.ItemKey] == nil {
			accepted[gap.ItemKey] = map[string]bool{}
		}
		accepted[gap.ItemKey][gap.Kind] = true
	}
	entries := make([]membership.Entry, 0, len(p.entries))
	for _, e := range p.entries {
		if (e.Disposition == "withdrawn" || e.Disposition == "scope_removed") && accepted[e.DocumentKey][e.Disposition] {
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}
