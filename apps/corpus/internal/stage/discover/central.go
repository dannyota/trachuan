package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"danny.vn/trachuan/apps/corpus/internal/lookup"
	"danny.vn/trachuan/apps/corpus/internal/source"
)

func (s *Stage) central(ctx context.Context, dir string, rules source.VBPLSources) error {
	identity := source.CentralListingIdentity("vbpl", "TRUNG_UONG", 1000)
	definition, _ := json.Marshal(map[string]any{"source": "vbpl", "type": "listing", "level": "TRUNG_UONG", "page_size": 1000})
	report := scopeReport{Version: 2, QueryIdentity: identity, Passes: []lookup.Result{}}
	union := map[string]bool{}
	var previous []string
	var final []lookup.Item
	passReferences := []map[string]pageReference{}
	for pass := 1; pass <= 3; pass++ {
		result, passRefs, err := s.centralPass(ctx, dir, pass, identity, definition)
		if err != nil {
			return err
		}
		report.Passes = append(report.Passes, result)
		current := ids(result.Items)
		for _, id := range current {
			union[id] = true
		}
		final = result.Items
		passReferences = append(passReferences, passRefs)
		if len(report.Passes) > 1 && result.Evidence.Completeness == lookup.Complete && report.Passes[len(report.Passes)-2].Evidence.Completeness == lookup.Complete && sameIDs(previous, current) {
			report.Stable = true
			break
		}
		previous = current
	}
	catalog := catalogFile{Complete: report.Stable}
	if report.Stable {
		catalog.IDs = ids(final)
	} else {
		for id := range union {
			catalog.IDs = append(catalog.IDs, id)
		}
		sort.Strings(catalog.IDs)
	}
	selectedByID := map[string]selectedItem{}
	first := 0
	if report.Stable {
		first = len(report.Passes) - 1
	}
	for i := first; i < len(report.Passes); i++ {
		for _, item := range report.Passes[i].Items {
			title := ""
			if item.Title != nil {
				title = *item.Title
			}
			choices, err := source.Select([]source.ListingItem{{ID: item.SourceID, Title: title, AgencyIDs: item.AgencyIDs}}, rules)
			if err != nil {
				return fmt.Errorf("select listing: %w", err)
			}
			if len(choices) != 0 {
				page := passReferences[i][item.SourceID]
				prior := selectedByID[item.SourceID]
				current := selectedItem{ID: item.SourceID, Number: item.RawNumber, Title: item.Title, IssueDate: item.IssueDate, Issuer: item.Issuer, AgencyIDs: item.AgencyIDs, Page: page}
				current.RulePages = append(current.RulePages, prior.RulePages...)
				rulesSeen := map[source.SelectionRule]bool{}
				for _, rp := range current.RulePages {
					rulesSeen[rp.Rule] = true
				}
				for _, rule := range choices[0].Rules {
					current.RulePages = append(current.RulePages, rulePage{Rule: rule, Page: page})
					rulesSeen[rule] = true
				}
				for rule := range rulesSeen {
					current.Rules = append(current.Rules, rule)
				}
				sort.Slice(current.Rules, func(a, b int) bool {
					x, y := current.Rules[a], current.Rules[b]
					if x.Kind != y.Kind {
						return x.Kind < y.Kind
					}
					return x.Value < y.Value
				})
				selectedByID[item.SourceID] = current
			}
		}
	}
	selected := selectedFile{Items: []selectedItem{}}
	keys := make([]string, 0, len(selectedByID))
	for key := range selectedByID {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		selected.Items = append(selected.Items, selectedByID[key])
	}
	for _, file := range []struct {
		name  string
		value any
	}{{"report.json", report}, {"catalog.json", catalog}, {"selected.json", selected}} {
		if err := writeJSON(dir, file.name, file.value); err != nil {
			return err
		}
	}
	return nil
}
func (s *Stage) centralPass(ctx context.Context, dir string, pass int, identity string, definition json.RawMessage) (result lookup.Result, references map[string]pageReference, err error) {
	pages := newCentralPages(ctx, s.client)
	defer func() {
		if pause := pages.close(); pause != nil {
			result, references, err = lookup.Result{}, nil, retryLookup(pause)
		}
	}()
	b := lookup.Builder{QueryIdentity: identity, Query: definition, Items: []lookup.Item{}, Pages: []lookup.Page{}}
	seen := map[string]bool{}
	refs := map[string]pageReference{}
	for page := 1; page <= 100001; page++ {
		p, err := pages.get(page)
		if err != nil {
			return lookup.Result{}, nil, retryLookup(err)
		}
		ref, err := writePage(dir, pass, page, p.Raw)
		if err != nil {
			return lookup.Result{}, nil, err
		}
		var envelope struct {
			Data struct {
				Total *int `json:"total"`
			} `json:"data"`
		}
		if err = json.Unmarshal(p.Raw, &envelope); err != nil {
			return lookup.Result{}, nil, fmt.Errorf("decode total: %w", err)
		}
		total := envelope.Data.Total
		if total == nil {
			b.Uncertain = true
		} else if *total > 100000 {
			return lookup.Result{}, nil, fmt.Errorf("central listing exceeds 100000 ids")
		}
		if b.TotalReported == nil {
			b.TotalReported = total
		}
		ordinal := page
		record := lookup.Page{Ordinal: &ordinal, Total: total, SHA256: ref.SHA256, Path: ref.Path, ResultIDs: []string{}}
		added := 0
		for _, item := range p.Items {
			record.ResultIDs = append(record.ResultIDs, item.ID)
			if !seen[item.ID] {
				seen[item.ID] = true
				added++
				refs[item.ID] = ref
				b.Items = append(b.Items, lookup.Item{Source: "vbpl", SourceID: item.ID, RawNumber: optional(item.DocNum), Title: optional(item.Title), IssueDate: optional(item.IssueDate), Issuer: optional(item.AgencyName), AgencyIDs: item.AgencyIDs, PageRef: ref.SHA256})
			}
		}
		b.Pages = append(b.Pages, record)
		if len(seen) > 100000 {
			return lookup.Result{}, nil, fmt.Errorf("central listing exceeds 100000 ids")
		}
		if len(p.Items) == 0 || (b.TotalReported != nil && len(seen) >= *b.TotalReported) {
			b.EndedNormally = true
			return b.Result(), refs, nil
		}
		if added == 0 {
			b.Uncertain = true
			return b.Result(), refs, nil
		}
	}
	return lookup.Result{}, nil, fmt.Errorf("central listing exceeds page limit")
}
func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
