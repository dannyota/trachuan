package discover

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/pinned"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/internal/retrieval"
	"github.com/jackc/pgx/v5"
)

func (*Stage) Preflight(ctx context.Context, env stage.Env, tx pgx.Tx, r stage.Run) (stage.Finalization, error) {
	list, err := source.Parse(r.Snapshot.SourceList)
	if err != nil {
		return stage.Finalization{}, fmt.Errorf("parse pin sources: %w", err)
	}
	if len(list.VBPL.Pinned) == 0 {
		return stage.Finalization{}, nil
	}
	if list.VBPL.SelectionVersion != 1 {
		return invalidPin(fmt.Errorf("pins require selection version 1")), nil
	}
	if err = pinned.Verify(ctx, store.New(tx), env.Artifacts, r.Country, list.VBPL.Pinned); err != nil {
		return invalidPin(err), nil
	}
	return stage.Finalization{}, nil
}

func invalidPin(err error) stage.Finalization {
	return stage.Finalization{Code: "pin_invalid", ItemKey: "vbpl:central", Reason: err.Error()}
}

func addPinnedMembers(r stage.Run, pins []source.Pin, members map[string]*seedMember) stage.Finalization {
	keys := make([]string, 0, len(members))
	for key := range members {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, pin := range pins {
		number := retrieval.NormalizeNumber(pin.Number)
		for _, key := range keys {
			m := members[key]
			matches := m.numbers[number] || (m.entry.NormalizedNumber != nil && *m.entry.NormalizedNumber == number)
			if key != pin.Key && m.entry.Source == "vbpl" && m.entry.Disposition == "active" && matches {
				return stage.Finalization{Code: "pin_conflict", ItemKey: "vbpl:central", Reason: fmt.Sprintf("pin %s conflicts with listed %s", pin.Key, key), EvidenceRefs: []string{r.Snapshot.SourceListHash, pin.Expand, pin.Fetch}}
			}
		}
	}
	for _, pin := range pins {
		m := members[pin.Key]
		if m == nil {
			number := retrieval.NormalizeNumber(pin.Number)
			m = &seedMember{entry: membership.Entry{DocumentKey: pin.Key, Source: "vbpl", SourceID: strings.TrimPrefix(pin.Key, r.Country+":vbpl:"), Class: "primary", Disposition: "active", RawNumber: &pin.Number, NormalizedNumber: &number}}
			members[pin.Key] = m
		}
		m.entry.Class, m.entry.Disposition = "primary", "active"
		number := retrieval.NormalizeNumber(pin.Number)
		m.entry.RawNumber, m.entry.NormalizedNumber = &pin.Number, &number
		identity := "source-list:" + r.Snapshot.SourceListHash
		support := membership.SupportTuple{RuleKind: "pinned", RuleValue: pin.Key, Evidence: membership.EvidenceRef{QueryIdentity: &identity, ResponseSHA256: pin.Expand, ArtifactHash: pin.Fetch, Path: "vbpl.pinned"}}
		m.entry.Supports = append(m.entry.Supports, support)
		m.records = append(m.records, membership.EvidenceRecord{DocumentKey: pin.Key, Stage: "discover", ItemKey: "vbpl:central", Support: support, State: "current"})
	}
	return stage.Finalization{}
}
