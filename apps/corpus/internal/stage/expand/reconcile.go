package expand

import (
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
)

type seedObservation struct {
	complete     bool
	artifactHash *string
	observation  Observation
}
type member struct {
	entry   membership.Entry
	records []membership.EvidenceRecord
}

func reconcile(seeds, prior []membership.Entry, observations map[string]seedObservation) map[string]*member {
	members := make(map[string]*member, len(seeds))
	active := map[string]bool{}
	for _, e := range seeds {
		entry := e
		members[e.DocumentKey] = &member{entry: entry}
		if e.Source == "vbpl" && e.Class == "primary" && e.Disposition == "active" && !strings.HasPrefix(strings.ToLower(e.SourceID), "vbpqta_") {
			active[e.DocumentKey] = true
		}
	}
	ensure := func(key string) *member {
		m := members[key]
		if m == nil {
			_, id, _ := strings.Cut(key, ":vbpl:")
			m = &member{entry: membership.Entry{DocumentKey: key, Source: "vbpl", SourceID: id, Class: "relation_context", Disposition: "active"}}
			members[key] = m
		}
		return m
	}
	add := func(m *member, rec membership.EvidenceRecord) {
		m.records = append(m.records, rec)
		m.entry.Supports = membership.MergeSupports(m.entry.Supports, []membership.SupportTuple{rec.Support})
	}
	for seed, o := range observations {
		if !o.complete || o.artifactHash == nil {
			continue
		}
		for _, t := range o.observation.Targets {
			m := ensure(t.TargetKey)
			if !active[t.TargetKey] {
				m.entry.Class = "relation_context"
			}
			m.entry.Disposition = "active"
			s := membership.SupportTuple{RuleKind: "relation", RuleValue: seed + "|" + t.RawCode, Evidence: membership.EvidenceRef{ResponseSHA256: t.ResponseSHA256, ArtifactHash: *o.artifactHash, Path: t.Response}}
			add(m, membership.EvidenceRecord{DocumentKey: t.TargetKey, Stage: "expand", ItemKey: seed, Support: s, State: "current"})
		}
	}
	for _, old := range prior {
		retainedRemoval := old.Disposition == "withdrawn" || old.Disposition == "scope_removed"
		if old.Disposition != "active" && !retainedRemoval {
			continue
		}
		wasActive := old.Disposition == "active"
		hasActiveSupport := false
		var records []membership.EvidenceRecord
		for _, s := range old.Supports {
			if s.RuleKind != "relation" {
				continue
			}
			seed, _, ok := strings.Cut(s.RuleValue, "|")
			if !ok {
				continue
			}
			state, owner, item := "removed", "discover", "vbpl:central"
			if active[seed] {
				hasActiveSupport = true
				owner, item = "expand", seed
				if wasActive && !observations[seed].complete {
					state = "retained"
				}
			}
			records = append(records, membership.EvidenceRecord{DocumentKey: old.DocumentKey, Stage: owner, ItemKey: item, Support: s, State: state})
		}
		m := members[old.DocumentKey]
		if m == nil && old.Class != "relation_context" && !retainedRemoval {
			retain := false
			for _, rec := range records {
				retain = retain || rec.State == "retained"
			}
			if !retain {
				continue
			}
		}
		if m == nil {
			entry := old
			entry.Supports = nil
			entry.ReplacementKey = nil
			entry.Class = "relation_context"
			if wasActive {
				if hasActiveSupport {
					entry.Disposition = "withdrawn"
				} else {
					entry.Disposition = "scope_removed"
				}
			}
			m = &member{entry: entry}
			members[old.DocumentKey] = m
		}
		if wasActive && old.Class == "relation_context" && m.entry.Disposition != "active" {
			if hasActiveSupport {
				m.entry.Disposition = "withdrawn"
			} else {
				m.entry.Disposition = "scope_removed"
			}
		}
		for _, rec := range records {
			duplicate := false
			for _, current := range m.records {
				if current.State == "current" && current.Support.RuleKind == rec.Support.RuleKind && current.Support.RuleValue == rec.Support.RuleValue {
					duplicate = true
					break
				}
			}
			if duplicate {
				continue
			}
			if rec.State == "retained" {
				if !active[old.DocumentKey] {
					m.entry.Class = "relation_context"
				}
				m.entry.Disposition = "active"
			}
			add(m, rec)
		}
	}
	return members
}
