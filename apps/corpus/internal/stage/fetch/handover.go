package fetch

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

type handOverFetched struct {
	identity handOverIdentity
	refs     []string
	done     bool
}

type handOverEvidence struct {
	secondary              handOverIdentity
	secondaryHistory       []handOverIdentity
	hasSecondary, conflict bool
	pairs                  map[string][]handOverObservation
	refs                   []string
}
type handOverObservation struct {
	identity handOverIdentity
	state    string
	complete bool
}

func (s *Stage) handOver(ctx context.Context, env stage.Env, q *store.Queries, run stage.Run, snapshot membership.PhaseSnapshot, rows []store.ListStageItemArtifactsRow) ([]membership.Entry, error) {
	entries := append([]membership.Entry(nil), snapshot.Entries...)
	hasSecondary := false
	for _, entry := range entries {
		if entry.Source == "chinhphu" && entry.Disposition == "active" {
			hasSecondary = true
			break
		}
	}
	if !hasSecondary {
		return entries, nil
	}
	items := map[string]store.ListStageItemArtifactsRow{}
	for _, row := range rows {
		items[row.ItemKey] = row
	}
	primary := map[string]handOverFetched{}
	for _, entry := range snapshot.Active {
		if entry.Source != "vbpl" {
			continue
		}
		item, ok := items[entry.DocumentKey]
		if !ok {
			return nil, fmt.Errorf("missing planned fetch item %s", entry.DocumentKey)
		}
		f := handOverFetched{}
		if item.State == "done" {
			var err error
			f.identity, f.refs, err = s.readVBPLHandOver(ctx, env, item)
			if err != nil {
				return nil, err
			}
			f.done = true
		}
		primary[entry.DocumentKey] = f
	}
	gaps, err := q.ListRunGapsByRun(ctx, run.ID)
	if err != nil {
		return nil, fmt.Errorf("list fetch gaps: %w", err)
	}
	itemGaps := map[string][]store.Gap{}
	for _, gap := range gaps {
		if gap.Stage == "fetch" {
			itemGaps[gap.ItemKey] = append(itemGaps[gap.ItemKey], gap)
		}
	}
	for i := range entries {
		entry := &entries[i]
		if entry.Source != "chinhphu" || entry.Disposition != "active" {
			continue
		}
		item, ok := items[entry.DocumentKey]
		if !ok {
			return nil, fmt.Errorf("missing planned fetch item %s", entry.DocumentKey)
		}
		records, err := readHandOverRecords(ctx, q, env, run, *entry)
		if err != nil {
			return nil, err
		}
		ev := handOverEvidence{pairs: map[string][]handOverObservation{}}
		for _, record := range records {
			ev.addRecord(record, primary)
		}
		if item.State == "done" {
			identity, refs, e := s.readSecondaryHandOver(ctx, env, item)
			if e != nil {
				return nil, e
			}
			for _, prior := range ev.secondaryHistory {
				if identityConflict(prior, identity) {
					ev.conflict = true
				}
			}
			ev.secondary = identity
			ev.hasSecondary = true
			ev.refs = append(ev.refs, refs...)
		}
		pairs := ev.pairOutcomes(primary)
		decision := decideHandOver(pairs)
		skipped := precedenceSkipped(item, itemGaps[entry.DocumentKey])
		if skipped {
			_, err = q.DeleteItemGaps(ctx, store.DeleteItemGapsParams{RunID: run.ID, Stage: "fetch", ItemKey: entry.DocumentKey})
			if err != nil {
				return nil, fmt.Errorf("delete provisional hand-over gap: %w", err)
			}
		}
		detail := entry.DocumentKey + " ~ " + strings.Join(decision.keys, ",")
		if decision.kind == "source_replaced" {
			entry.Disposition = "source_replaced"
			key := decision.keys[0]
			entry.ReplacementKey = &key
			detail = entry.DocumentKey + " -> " + key
		}
		sort.Strings(ev.refs)
		refs := uniqueHandOverRefs(ev.refs)
		evidence := strings.Join(refs, ";")
		source, origin := "chinhphu", "source_data"
		finalized := false
		for _, gap := range itemGaps[entry.DocumentKey] {
			if gap.EvidenceRef == nil || (gap.Kind != "source_replaced" && gap.Kind != "source_identity_ambiguous") {
				continue
			}
			if finalized || gap.Kind != decision.kind || gap.Detail != detail || *gap.EvidenceRef != evidence || gap.Source == nil || *gap.Source != source || gap.Origin == nil || *gap.Origin != origin {
				return nil, fmt.Errorf("hand-over outcome changed after finalization")
			}
			finalized = true
		}
		if finalized || decision.kind == "" {
			continue
		}
		if err = q.InsertGap(ctx, store.InsertGapParams{RunID: run.ID, Stage: "fetch", ItemKey: entry.DocumentKey, Kind: decision.kind, Detail: detail, Source: &source, Origin: &origin, EvidenceRef: &evidence}); err != nil {
			return nil, fmt.Errorf("insert hand-over gap: %w", err)
		}
	}
	return entries, nil
}
func uniqueHandOverRefs(refs []string) []string {
	out := refs[:0]
	for _, ref := range refs {
		if len(out) == 0 || out[len(out)-1] != ref {
			out = append(out, ref)
		}
	}
	return out
}
func precedenceSkipped(item store.ListStageItemArtifactsRow, gaps []store.Gap) bool {
	return item.State == "gap" && len(gaps) == 1 && gaps[0].Kind == "source_replaced" && gaps[0].EvidenceRef == nil
}
