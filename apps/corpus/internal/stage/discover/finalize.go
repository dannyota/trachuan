package discover

import (
	"context"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

// Finalize publishes the discovery seed membership in the close transaction.
func (*Stage) Finalize(ctx context.Context, env stage.Env, tx pgx.Tx, r stage.Run) (stage.Finalization, error) {
	list, err := source.Parse(r.Snapshot.SourceList)
	if err != nil {
		return stage.Finalization{}, fmt.Errorf("parse discovery sources: %w", err)
	}
	if list.VBPL.SelectionVersion == 0 {
		return stage.Finalization{}, nil
	}
	if list.VBPL.SelectionVersion != 1 {
		return stage.Finalization{}, fmt.Errorf("unsupported selection version %d", list.VBPL.SelectionVersion)
	}
	q := store.New(tx)
	members, err := currentMembers(ctx, q, r)
	if err != nil {
		return stage.Finalization{}, err
	}
	ambiguous, err := supplementNamed(ctx, q, env, r, list.VBPL.NamedNumbers, members)
	if err != nil {
		return stage.Finalization{}, err
	}
	central, err := readObservation(ctx, q, env, r.ID, "vbpl:central", true)
	if err != nil {
		return stage.Finalization{}, err
	}
	configured, complete := map[string]bool{}, map[string]bool{}
	for _, name := range list.VBPL.NamedNumbers {
		configured[name] = true
		o, e := readObservation(ctx, q, env, r.ID, "vbpl:named:"+name, false)
		if e != nil {
			return stage.Finalization{}, e
		}
		complete[name] = o.complete
	}
	history, err := earlierRuns(ctx, q, r)
	if err != nil {
		return stage.Finalization{}, err
	}
	prior, err := earlierMembership(ctx, q, history, r.Country)
	if err != nil {
		return stage.Finalization{}, err
	}
	previous, err := earlierCatalog(ctx, q, env, history)
	if err != nil {
		return stage.Finalization{}, err
	}
	for _, entry := range prior {
		if entry.Source != "vbpl" || englishRendition(entry.DocumentKey) {
			continue
		}
		var direct []membership.SupportTuple
		for _, s := range entry.Supports {
			if directSupport(s) {
				direct = append(direct, s)
			}
		}
		if len(direct) == 0 {
			continue
		}
		m := members[entry.DocumentKey]
		if m == nil {
			entry.Supports = nil
			entry.ReplacementKey = nil
			entry.Disposition = "active"
			m = &seedMember{entry: entry}
			members[entry.DocumentKey] = m
		}
		hasCurrent := len(m.records) > 0
		retained := false
		var old []membership.EvidenceRecord
		for _, s := range direct {
			if superseded(s, m.records) {
				continue
			}
			state := priorState(s, central.complete, configured, complete)
			if s.RuleKind == "named" && ambiguous[s.RuleValue][entry.SourceID] {
				state = "retained"
			}
			retained = retained || state == "retained"
			old = append(old, membership.EvidenceRecord{DocumentKey: entry.DocumentKey, Stage: "discover", ItemKey: "vbpl:central", Support: s, State: state})
		}
		if !hasCurrent && !retained {
			switch {
			case central.complete && central.catalog[entry.SourceID]:
				m.entry.Disposition = "scope_removed"
			case central.complete && !central.catalog[entry.SourceID] && previous.complete && !previous.catalog[entry.SourceID]:
				m.entry.Disposition = "withdrawn"
			default:
				for i := range old {
					old[i].State = "retained"
				}
			}
		}
		for _, record := range old {
			m.entry.Supports = append(m.entry.Supports, record.Support)
			m.records = append(m.records, record)
		}
	}
	entries := make([]membership.Entry, 0, len(members))
	for _, m := range members {
		m.entry.Supports = membership.MergeSupports(nil, m.entry.Supports)
		entries = append(entries, m.entry)
	}
	frozen, err := verifyFrozenEvidence(ctx, q, r.ID, members)
	if err != nil {
		return stage.Finalization{}, err
	}
	if err = membership.PublishPhase(ctx, q, r.ID, membership.Seeds, entries); err != nil {
		return stage.Finalization{}, fmt.Errorf("publish seeds: %w", err)
	}
	if frozen {
		return stage.Finalization{}, nil
	}
	for _, m := range members {
		if err = membership.UpsertDocument(ctx, q, r.ID, m.entry); err != nil {
			return stage.Finalization{}, err
		}
		for _, record := range m.records {
			if err = membership.RecordEvidence(ctx, q, r.ID, record); err != nil {
				return stage.Finalization{}, err
			}
		}
	}
	return stage.Finalization{}, nil
}
