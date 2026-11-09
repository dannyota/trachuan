package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/internal/retrieval"
)

type seedMember struct {
	entry   membership.Entry
	records []membership.EvidenceRecord
}

func currentMembers(ctx context.Context, q *store.Queries, r stage.Run) (map[string]*seedMember, error) {
	rows, err := q.ListRunListings(ctx, r.ID)
	if err != nil {
		return nil, fmt.Errorf("list current listings: %w", err)
	}
	members := map[string]*seedMember{}
	artifacts := map[string]store.GetRunItemArtifactRow{}
	for _, row := range rows {
		if englishRendition(row.ItemKey) {
			continue
		}
		var selected selectedItem
		if err = json.Unmarshal(row.Payload, &selected); err != nil {
			return nil, fmt.Errorf("decode listing %s: %w", row.ItemKey, err)
		}
		prefix := r.Country + ":vbpl:"
		if row.ItemKey != prefix+selected.ID || selected.ID == "" {
			return nil, fmt.Errorf("invalid listing key %q", row.ItemKey)
		}
		m := members[row.ItemKey]
		if m == nil {
			m = &seedMember{entry: membership.Entry{DocumentKey: row.ItemKey, Source: "vbpl", SourceID: selected.ID, Class: "primary", Disposition: "active"}}
			members[row.ItemKey] = m
		}
		if selected.Number != nil {
			n := retrieval.NormalizeNumber(*selected.Number)
			m.entry.RawNumber = selected.Number
			m.entry.NormalizedNumber = &n
		}
		for _, rule := range selected.Rules {
			item := "vbpl:central"
			switch rule.Kind {
			case "agency", "term":
			case "named":
				item = "vbpl:named:" + rule.Value
			default:
				return nil, fmt.Errorf("invalid selection rule %q", rule.Kind)
			}
			art, ok := artifacts[item]
			if !ok {
				art, err = q.GetRunItemArtifact(ctx, store.GetRunItemArtifactParams{RunID: r.ID, Stage: "discover", ItemKey: item})
				if err != nil {
					return nil, fmt.Errorf("read producing item %s: %w", item, err)
				}
				artifacts[item] = art
			}
			identity := row.QueryKey
			page := fmt.Sprintf("%d:%d", selected.Page.Pass, selected.Page.Page)
			tuple := membership.SupportTuple{RuleKind: rule.Kind, RuleValue: rule.Value, Evidence: membership.EvidenceRef{QueryIdentity: &identity, Page: &page, ResponseSHA256: selected.Page.SHA256, ArtifactHash: art.Sha256, Path: selected.Page.Path}}
			m.entry.Supports = append(m.entry.Supports, tuple)
			m.records = append(m.records, membership.EvidenceRecord{DocumentKey: row.ItemKey, Stage: "discover", ItemKey: item, Support: tuple, State: "current"})
		}
	}
	return members, nil
}
func isLegacy(s membership.SupportTuple) bool {
	return s.Evidence.QueryIdentity != nil && (strings.HasPrefix(*s.Evidence.QueryIdentity, "vbpl:sweep@") || strings.HasPrefix(*s.Evidence.QueryIdentity, "vbpl:keyword:"))
}
func directSupport(s membership.SupportTuple) bool {
	return s.RuleKind == "agency" || s.RuleKind == "term" || s.RuleKind == "named" || isLegacy(s)
}
func sameSupport(a, b membership.SupportTuple) bool {
	return len(membership.MergeSupports([]membership.SupportTuple{a}, []membership.SupportTuple{b})) == 1
}
func priorState(s membership.SupportTuple, central bool, configured, complete map[string]bool) string {
	if isLegacy(s) {
		return "removed"
	}
	switch s.RuleKind {
	case "agency", "term":
		if !central {
			return "retained"
		}
	case "named":
		if configured[s.RuleValue] && !complete[s.RuleValue] {
			return "retained"
		}
	}
	return "removed"
}

func superseded(old membership.SupportTuple, records []membership.EvidenceRecord) bool {
	for _, record := range records {
		if record.State == "current" && record.Support.RuleKind == old.RuleKind && record.Support.RuleValue == old.RuleValue {
			return true
		}
	}
	return false
}
