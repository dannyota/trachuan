package discover

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/internal/retrieval"
)

func addSecondaryMembers(ctx context.Context, q *store.Queries, env stage.Env, r stage.Run, obs []secondaryObservation, members map[string]*seedMember) error {
	for _, o := range obs {
		for _, c := range o.report.Candidates {
			if c.State == "conflicting_detail" {
				if err := q.InsertGap(ctx, store.InsertGapParams{RunID: r.ID, Stage: "discover", ItemKey: o.key, Kind: "invalid_source_data", Detail: "conflicting secondary detail " + c.ID}); err != nil {
					return fmt.Errorf("record conflicting detail: %w", err)
				}
				continue
			}
			if c.State != "admit_secondary" && c.State != "seed_primary" && c.State != "ambiguous" {
				continue
			}
			src, id, class := "chinhphu", c.ID, "secondary"
			if c.State == "seed_primary" {
				src, id, class = "vbpl", c.PrimaryID, "primary"
			}
			if id == "" {
				return fmt.Errorf("secondary candidate %s lacks target", c.ID)
			}
			key := r.Country + ":" + src + ":" + id
			m := members[key]
			if m == nil {
				m = &seedMember{entry: membership.Entry{DocumentKey: key, Source: src, SourceID: id, Class: class, Disposition: "active"}}
				members[key] = m
			}
			number := c.Number
			norm := retrieval.NormalizeNumber(number)
			if m.entry.RawNumber == nil {
				m.entry.RawNumber = &number
				m.entry.NormalizedNumber = &norm
			}
			path := "candidate-" + c.ID + ".json"
			row, err := q.GetRunItemArtifact(ctx, store.GetRunItemArtifactParams{RunID: r.ID, Stage: "discover", ItemKey: o.key})
			if err != nil {
				return fmt.Errorf("read candidate artifact: %w", err)
			}
			b, err := os.ReadFile(filepath.Join(env.Artifacts.Abs(row.Path), path))
			if err != nil {
				return fmt.Errorf("read candidate evidence: %w", err)
			}
			hash := sha256.Sum256(b)
			identity := o.report.QueryIdentity
			tuple := membership.SupportTuple{RuleKind: "secondary_probe", RuleValue: o.key + "|" + c.ID, Evidence: membership.EvidenceRef{QueryIdentity: &identity, ResponseSHA256: fmt.Sprintf("%x", hash), ArtifactHash: o.hash, Path: path}}
			m.entry.Supports = append(m.entry.Supports, tuple)
			m.records = append(m.records, membership.EvidenceRecord{DocumentKey: key, Stage: "discover", ItemKey: o.key, Support: tuple, State: "current"})
		}
	}
	return nil
}
func secondaryObservationMap(obs []secondaryObservation) map[string]secondaryObservation {
	m := map[string]secondaryObservation{}
	for _, o := range obs {
		m[o.key] = o
	}
	return m
}
func retainSecondary(prior []membership.Entry, obs map[string]secondaryObservation, members map[string]*seedMember, histories ...map[string]secondaryHistory) {
	history := map[string]secondaryHistory{}
	if len(histories) > 0 {
		history = histories[0]
	}
	for _, entry := range prior {
		if entry.Source != "chinhphu" || (entry.Disposition != "active" && entry.Disposition != "withdrawn" && entry.Disposition != "scope_removed") {
			continue
		}
		supports := entry.Supports
		m := members[entry.DocumentKey]
		if m == nil {
			entry.Supports = nil
			entry.ReplacementKey = nil
			m = &seedMember{entry: entry}
			members[entry.DocumentKey] = m
		}
		current := len(m.records) > 0
		retained := false
		var old []membership.EvidenceRecord
		for _, s := range supports {
			if s.RuleKind != "secondary_probe" || superseded(s, m.records) {
				continue
			}
			state := secondaryPriorState(s, obs)
			key, id, _ := strings.Cut(s.RuleValue, "|")
			if o, ok := obs[key]; ok {
				for _, candidate := range o.report.Candidates {
					if candidate.ID == id && candidate.State == "seed_primary" {
						state = "retained"
					}
				}
			}
			if entry.Disposition != "active" {
				state = "removed"
			}
			retained = retained || state == "retained"
			old = append(old, membership.EvidenceRecord{DocumentKey: entry.DocumentKey, Stage: "discover", ItemKey: key, Support: s, State: state})
		}
		if entry.Disposition == "active" && !current && !retained {
			configured, allAbsent, previousAbsent := false, true, true
			for _, record := range old {
				key, id, ok := strings.Cut(record.Support.RuleValue, "|")
				if !ok {
					continue
				}
				o, ok := obs[key]
				if !ok || (record.Support.Evidence.QueryIdentity != nil && o.report.QueryIdentity != "" && *record.Support.Evidence.QueryIdentity != o.report.QueryIdentity) {
					continue
				}
				configured = true
				if secondarySelectedIDs(o.report)[id] {
					allAbsent = false
				}
				h, ok := history[key]
				if !ok || h.present[id] {
					previousAbsent = false
				}
			}
			switch {
			case !configured || !allAbsent:
				m.entry.Disposition = "scope_removed"
			case previousAbsent:
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
}

func markSecondaryConflicts(obs []secondaryObservation) {
	type identity struct{ number, issuer, date string }
	identities := map[string][]identity{}
	conflicts := map[string]bool{}
	for _, o := range obs {
		for _, c := range o.report.Candidates {
			x := identity{retrieval.NormalizeNumber(c.Number), source.NormalizeIssuer(c.IssuerForm), c.Date}
			for _, old := range identities[c.ID] {
				if old.number != "" && x.number != "" && old.number != x.number || old.issuer != "" && x.issuer != "" && old.issuer != x.issuer || old.date != "" && x.date != "" && old.date != x.date {
					conflicts[c.ID] = true
				}
			}
			identities[c.ID] = append(identities[c.ID], x)
		}
	}
	for i := range obs {
		for j := range obs[i].report.Candidates {
			c := &obs[i].report.Candidates[j]
			if conflicts[c.ID] {
				c.State = "conflicting_detail"
			}
		}
	}
}
