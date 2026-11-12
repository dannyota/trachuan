package evaluation

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"danny.vn/trachuan/internal/retrieval"
)

// FilterGate contains the filter ordering checks for both retrieval modes.
type FilterGate struct {
	Modes   []Floor                            `json:"modes"`
	Cohorts map[string]map[string]FilterCohort `json:"cohorts"`
	Cases   []FilterCase                       `json:"cases"`
	Reason  string                             `json:"reason,omitempty"`
}

// FilterCohort counts evaluated and skipped cases.
type FilterCohort struct {
	CaseCount int            `json:"case_count"`
	Passed    int            `json:"passed"`
	Skipped   map[string]int `json:"skipped"`
}

// FilterCase records one filtered result without its query.
type FilterCase struct {
	CaseID            string         `json:"case_id"`
	Mode              retrieval.Mode `json:"mode"`
	Cohort            string         `json:"cohort"`
	Filter            []string       `json:"filter"`
	FilteredRecall8   *float64       `json:"filtered_recall8"`
	UnfilteredRecall8 *float64       `json:"unfiltered_recall8"`
	Within100         *bool          `json:"within_100"`
	Passed            bool           `json:"passed"`
	SkipReason        string         `json:"skip_reason"`
}

type filterEngine interface {
	Taxonomy(context.Context, retrieval.TaxonomyRequest) (retrieval.TaxonomyResult, error)
	AnchorNodes(context.Context, []retrieval.AnchorRef) (map[retrieval.AnchorRef][]string, error)
}
type filterSelection struct {
	cohort string
	filter []string
	skip   string
}
type filterState struct {
	gate     FilterGate
	families []string
	nodes    map[retrieval.AnchorRef][]string
	active   bool
}

func newFilterState(ctx context.Context, engine Engine, suite Suite, o Options) (filterState, error) {
	s := filterState{gate: FilterGate{Cohorts: map[string]map[string]FilterCohort{}, Cases: []FilterCase{}}}
	status := "out_of_scope"
	if o.CapOverride != nil || o.NonCurrentTail != nil || o.HistoricalPolicy != nil || o.NotYetEligible {
		status = "not_run"
		s.gate.Reason = "research_override"
	} else {
		taxonomy, ok := engine.(filterEngine)
		if !ok {
			return s, fmt.Errorf("filter gate requires taxonomy engine")
		}
		tree, err := taxonomy.Taxonomy(ctx, retrieval.TaxonomyRequest{Depth: 2})
		if err != nil {
			return s, fmt.Errorf("load filter taxonomy: %w", err)
		}
		if tree.Truncated {
			tree, err = taxonomy.Taxonomy(ctx, retrieval.TaxonomyRequest{Depth: 1})
			if err != nil {
				return s, fmt.Errorf("load filter taxonomy roots: %w", err)
			}
			if tree.Truncated {
				return s, fmt.Errorf("filter taxonomy roots truncated")
			}
			for i := range tree.Nodes {
				children, childErr := taxonomy.Taxonomy(ctx, retrieval.TaxonomyRequest{Node: tree.Nodes[i].Code, Depth: 1})
				if childErr != nil {
					return s, fmt.Errorf("load filter taxonomy families: %w", childErr)
				}
				if children.Truncated {
					return s, fmt.Errorf("filter taxonomy families truncated")
				}
				tree.Nodes[i].Children = children.Nodes
			}
		}
		for _, root := range tree.Nodes {
			for _, family := range root.Children {
				s.families = append(s.families, family.Code)
			}
		}
		if len(tree.Nodes) > 0 {
			status = "unavailable"
			s.active = true
		}
	}
	for _, mode := range []retrieval.Mode{"exact", "ann"} {
		s.gate.Modes = append(s.gate.Modes, Floor{Metric: Metric{Mode: mode, Arms: "hybrid", Limit: 8, Name: "filter_gate", ValidityOverrides: o.ValidityOverrides, CapOverride: o.CapOverride}, Floor: 1, Status: status})
		s.gate.Cohorts[string(mode)] = map[string]FilterCohort{}
		for _, name := range []string{"inside", "partial", "outside"} {
			s.gate.Cohorts[string(mode)][name] = FilterCohort{Skipped: map[string]int{}}
		}
	}
	if !s.active {
		return s, nil
	}
	var refs []retrieval.AnchorRef
	for _, e := range suite.Map.Entries {
		for _, a := range e.Alternatives {
			if a.Kind == "provision" && a.Anchor != nil {
				refs = append(refs, retrieval.AnchorRef{DocumentKey: a.DocumentKey, Anchor: *a.Anchor})
			}
		}
	}
	var err error
	s.nodes, err = engine.(filterEngine).AnchorNodes(ctx, refs)
	if err != nil {
		return s, fmt.Errorf("load filter anchor nodes: %w", err)
	}
	return s, nil
}

func family(code string) string {
	parts := strings.Split(code, ".")
	if len(parts) < 2 {
		return code
	}
	return strings.Join(parts[:2], ".")
}
func codeLess(a, b string) bool {
	aa, bb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(aa) && i < len(bb); i++ {
		if aa[i] == bb[i] {
			continue
		}
		x, xe := strconv.Atoi(aa[i])
		y, ye := strconv.Atoi(bb[i])
		if xe == nil && ye == nil {
			return x < y
		}
		return aa[i] < bb[i]
	}
	return len(aa) < len(bb)
}
func sortedCodes(values []string) []string {
	out := append([]string(nil), values...)
	slices.SortFunc(out, func(a, b string) int {
		if a == b {
			return 0
		}
		if codeLess(a, b) {
			return -1
		}
		return 1
	})
	return slices.Compact(out)
}

func deriveFilterCases(nodes [][]string, families []string) []filterSelection {
	var all []string
	seeded := 0
	for _, codes := range nodes {
		if len(codes) > 0 {
			seeded++
		}
		all = append(all, codes...)
	}
	all = sortedCodes(all)
	var out []filterSelection
	if seeded > 0 {
		cohort := "partial"
		if seeded == len(nodes) {
			cohort = "inside"
		}
		selected := all
		if len(selected) > 20 {
			var fs []string
			for _, code := range selected {
				fs = append(fs, family(code))
			}
			selected = sortedCodes(fs)
		}
		if len(selected) > 20 {
			out = append(out, filterSelection{cohort: cohort, skip: "too_many_families"})
		} else {
			out = append(out, filterSelection{cohort: cohort, filter: selected})
		}
	}
	occupied := map[string]bool{}
	for _, code := range all {
		occupied[family(code)] = true
	}
	outside := filterSelection{cohort: "outside", skip: "no_outside_family"}
	for _, code := range sortedCodes(families) {
		if !occupied[code] {
			outside.filter = []string{code}
			outside.skip = ""
			break
		}
	}
	return append(out, outside)
}

func (s *filterState) addCase(c Case, entries map[int]MapEntry, arms retrieval.Arms, recipe retrieval.Recipe, req retrieval.Request) error {
	if !s.active || c.ExpectAbstain || len(c.ExpectedCitations) == 0 {
		return nil
	}
	nodes := make([][]string, len(c.ExpectedCitations))
	seeded := make([]bool, len(nodes))
	for i := range nodes {
		entry, ok := entries[i]
		if !ok || entry.State != "resolved" {
			return nil
		}
		for _, a := range entry.Alternatives {
			if a.Kind == "provision" && a.Anchor != nil {
				nodes[i] = append(nodes[i], s.nodes[retrieval.AnchorRef{DocumentKey: a.DocumentKey, Anchor: *a.Anchor}]...)
			}
		}
		seeded[i] = len(nodes[i]) > 0
	}
	plain, err := retrieval.Shape(arms, recipe, 8, "hybrid")
	if err != nil {
		return fmt.Errorf("shape filter baseline: %w", err)
	}
	baseline := append(append([]retrieval.Hit{}, plain.Hits...), plain.NonCurrent...)
	for _, selection := range deriveFilterCases(nodes, s.families) {
		row := FilterCase{CaseID: c.ID, Mode: req.Mode, Cohort: selection.cohort, Filter: selection.filter, SkipReason: selection.skip}
		cohort := s.gate.Cohorts[string(req.Mode)][selection.cohort]
		cohort.CaseCount++
		if selection.skip != "" {
			cohort.Skipped[selection.skip]++
		} else {
			req.Nodes = selection.filter
			req.Arms = "hybrid"
			pages, pageErr := retrieval.PagedHits(arms, recipe, req, 8, 100)
			if pageErr != nil {
				return fmt.Errorf("page filter case %s: %w", c.ID, pageErr)
			}
			first, all := filterPageHits(pages)
			scoreFilterCase(&row, entries, seeded, first, baseline, all)
			if row.Passed {
				cohort.Passed++
			}
		}
		s.gate.Cohorts[string(req.Mode)][selection.cohort] = cohort
		s.gate.Cases = append(s.gate.Cases, row)
	}
	return nil
}

func filterPageHits(pages []retrieval.Response) ([]retrieval.Hit, []retrieval.Hit) {
	var first, all []retrieval.Hit
	for i, page := range pages {
		hits := append(append([]retrieval.Hit{}, page.Hits...), page.NonCurrent...)
		if i == 0 {
			first = hits
		}
		all = append(all, hits...)
	}
	if len(all) > 100 {
		all = all[:100]
	}
	return first, all
}

func scoreFilterCase(row *FilterCase, entries map[int]MapEntry, seeded []bool, first, baseline, all []retrieval.Hit) {
	filtered, unfiltered, seedFiltered, seedUnfiltered, matched := 0, 0, 0, 0, 0
	for i := range seeded {
		f := MatchProduct(entries[i], first) > 0
		u := MatchProduct(entries[i], baseline) > 0
		if f {
			filtered++
			if seeded[i] {
				seedFiltered++
			}
		}
		if u {
			unfiltered++
			if seeded[i] {
				seedUnfiltered++
			}
		}
		if MatchProduct(entries[i], all) > 0 {
			matched++
		}
	}
	fr, ur := float64(filtered)/float64(len(seeded)), float64(unfiltered)/float64(len(seeded))
	within := matched == len(seeded)
	row.FilteredRecall8 = &fr
	row.UnfilteredRecall8 = &ur
	row.Within100 = &within
	row.Passed = within && (row.Cohort == "outside" || row.Cohort == "inside" && filtered >= unfiltered || row.Cohort == "partial" && seedFiltered >= seedUnfiltered)
}

func (s *filterState) finish(modes []retrieval.Mode) {
	if !s.active {
		return
	}
	for i := range s.gate.Modes {
		m := &s.gate.Modes[i]
		if !slices.Contains(modes, m.Mode) {
			continue
		}
		cs := s.gate.Cohorts[string(m.Mode)]
		total, passed := 0, 0
		for _, c := range cs {
			skipped := 0
			for _, n := range c.Skipped {
				skipped += n
			}
			total += c.CaseCount - skipped
			passed += c.Passed
		}
		m.Numerator = float64(passed)
		m.Denominator = total
		if total > 0 {
			v := float64(passed) / float64(total)
			m.Value = &v
		}
		inside := cs["inside"]
		skipped := 0
		for _, n := range inside.Skipped {
			skipped += n
		}
		if inside.CaseCount-skipped == 0 {
			m.Status = "unavailable"
		} else if passed == total {
			m.Status = "pass"
		} else {
			m.Status = "fail"
		}
	}
}
