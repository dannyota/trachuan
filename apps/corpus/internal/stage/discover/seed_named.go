package discover

import (
	"context"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/lookup"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

// Named entries with different qualifiers can share the listing primary key.
// Recover their independent support from the individual lookup artifacts.
func supplementNamed(ctx context.Context, q *store.Queries, env stage.Env, r stage.Run, names []string, members map[string]*seedMember) (map[string]map[string]bool, error) {
	ambiguous := map[string]map[string]bool{}
	for _, name := range names {
		obs, err := readObservation(ctx, q, env, r.ID, "vbpl:named:"+name, false)
		if err != nil {
			return nil, err
		}
		if !obs.complete {
			continue
		}
		entry, err := source.ParseNamedNumber(name)
		if err != nil {
			return nil, fmt.Errorf("parse named entry: %w", err)
		}
		dir := env.Artifacts.Abs(obs.row.Path)
		var report scopeReport
		if err = readJSON(dir, "report.json", &report); err != nil {
			return nil, err
		}
		var result lookup.Result
		if err = readJSON(dir, "lookup.json", &result); err != nil {
			return nil, err
		}
		identity := source.NamedLookupIdentity("vbpl", "docNum", "exact", "TRUNG_UONG", entry.Number)
		if report.QueryIdentity != identity || result.Evidence.QueryIdentity != identity || result.Evidence.Completeness != lookup.Complete {
			return nil, fmt.Errorf("invalid named lookup evidence for %q", name)
		}
		eligible, unresolved := namedCandidates(result.Items, entry)
		if len(eligible) != 1 || len(unresolved) != 0 {
			ambiguous[name] = map[string]bool{}
			for _, candidate := range append(eligible, unresolved...) {
				ambiguous[name][candidate.SourceID] = true
			}
			continue
		}
		item := eligible[0]
		m := members[r.Country+":vbpl:"+item.SourceID]
		if m == nil {
			continue
		}
		ref := pageFor(result, item.PageRef, len(report.Passes))
		page := fmt.Sprintf("%d:%d", ref.Pass, ref.Page)
		tuple := membership.SupportTuple{RuleKind: "named", RuleValue: name, Evidence: membership.EvidenceRef{QueryIdentity: &identity, Page: &page, ResponseSHA256: ref.SHA256, ArtifactHash: obs.row.Sha256, Path: ref.Path}}
		found := false
		for _, record := range m.records {
			if record.ItemKey == "vbpl:named:"+name && sameSupport(record.Support, tuple) {
				found = true
				break
			}
		}
		if found {
			continue
		}
		m.entry.Supports = append(m.entry.Supports, tuple)
		m.records = append(m.records, membership.EvidenceRecord{DocumentKey: m.entry.DocumentKey, Stage: "discover", ItemKey: "vbpl:named:" + name, Support: tuple, State: "current"})
	}
	return ambiguous, nil
}
