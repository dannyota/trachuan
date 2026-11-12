package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

func TestFilterGate_OutOfScopeAndOverride(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: 11, Day: 9}
	cap, err := retrieval.ParseCapOverride("4")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		cap    *retrieval.CapOverride
		status string
	}{
		{"no served nodes", nil, "out_of_scope"},
		{"research override", cap, "not_run"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report, _, err := Run(context.Background(), &fakeEngine{date: date}, recordedSuite(), Options{EvaluationDate: date, CapOverride: tc.cap})
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				FilterGate struct {
					Modes []struct {
						Status string `json:"status"`
					} `json:"modes"`
					Reason string `json:"reason"`
				} `json:"filter_gate"`
			}
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			if len(got.FilterGate.Modes) != 2 {
				t.Fatalf("filter gate modes: got %d, want 2", len(got.FilterGate.Modes))
			}
			for _, mode := range got.FilterGate.Modes {
				if mode.Status != tc.status {
					t.Fatalf("status = %q, want %q", mode.Status, tc.status)
				}
			}
			if tc.cap != nil && got.FilterGate.Reason != "research_override" {
				t.Fatalf("reason = %q", got.FilterGate.Reason)
			}
		})
	}
}

type filterFixtureEngine struct {
	recordedEngine
	nodes    map[retrieval.AnchorRef][]string
	families []retrieval.NodeEntry
}

func (e *filterFixtureEngine) Taxonomy(_ context.Context, _ retrieval.TaxonomyRequest) (retrieval.TaxonomyResult, error) {
	return retrieval.TaxonomyResult{Nodes: e.families}, nil
}
func (e *filterFixtureEngine) AnchorNodes(_ context.Context, _ []retrieval.AnchorRef) (map[retrieval.AnchorRef][]string, error) {
	return e.nodes, nil
}
func (e *filterFixtureEngine) Collect(ctx context.Context, p retrieval.Prepared, d retrieval.Depths) (retrieval.Arms, error) {
	a, err := e.recordedEngine.Collect(ctx, p, d)
	a.EffectiveNodes = map[int64][]string{1: {"1.1.c1"}}
	return a, err
}

func TestFilterGate_CohortsAndQuality(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: 11, Day: 9}
	one := "one"
	for _, tc := range []struct {
		name     string
		nodes    []string
		families []retrieval.NodeEntry
		status   string
		cohorts  []string
		quality  bool
	}{
		{"inside and outside", []string{"1.1.c1"}, []retrieval.NodeEntry{{Code: "1", Children: []retrieval.NodeEntry{{Code: "1.1"}, {Code: "1.2"}}}}, "pass", []string{"inside", "outside"}, true},
		{"outside only", nil, []retrieval.NodeEntry{{Code: "1", Children: []retrieval.NodeEntry{{Code: "1.1"}}}}, "unavailable", []string{"outside"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			suite := recordedSuite()
			suite.Map.Entries[0].Alternatives = []Alternative{{Country: "vn", DocumentKey: "doc", Kind: "provision", Anchor: &one}}
			engine := &filterFixtureEngine{recordedEngine: recordedEngine{fakeEngine: fakeEngine{date: date}}, nodes: map[retrieval.AnchorRef][]string{{DocumentKey: "doc", Anchor: "one"}: tc.nodes}, families: tc.families}
			report, _, err := Run(context.Background(), engine, suite, Options{EvaluationDate: date})
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Quality bool `json:"quality_pass"`
				Gate    struct {
					Modes []struct {
						Status string `json:"status"`
					} `json:"modes"`
					Cases []struct {
						CaseID string `json:"case_id"`
						Cohort string `json:"cohort"`
					} `json:"cases"`
				} `json:"filter_gate"`
			}
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatal(err)
			}
			if got.Quality != tc.quality || len(got.Gate.Modes) != 2 {
				t.Fatalf("quality %v modes %d", got.Quality, len(got.Gate.Modes))
			}
			for _, m := range got.Gate.Modes {
				if m.Status != tc.status {
					t.Fatalf("status %s", m.Status)
				}
			}
			if len(got.Gate.Cases) != 2*len(tc.cohorts) {
				t.Fatalf("cases %d", len(got.Gate.Cases))
			}
			for _, c := range got.Gate.Cases {
				if c.CaseID != "positive" {
					t.Fatalf("case %s", c.CaseID)
				}
			}
		})
	}
}

func TestFilterGate_Derivation(t *testing.T) {
	sameFamily := make([]string, 21)
	for i := range sameFamily {
		sameFamily[i] = fmt.Sprintf("2.1.c%d", i+1)
	}
	many := make([]string, 21)
	for i := range many {
		many[i] = fmt.Sprintf("%d.1.c1", i+1)
	}
	for _, tc := range []struct {
		name     string
		nodes    [][]string
		families []string
		want     []filterSelection
	}{
		{"inside numeric outside", [][]string{{"2.1.c1"}, {"2.1.c2"}}, []string{"10.1", "2.1", "3.1"}, []filterSelection{{cohort: "inside", filter: []string{"2.1.c1", "2.1.c2"}}, {cohort: "outside", filter: []string{"3.1"}}}},
		{"partial", [][]string{{"2.1.c1"}, nil}, []string{"2.1"}, []filterSelection{{cohort: "partial", filter: []string{"2.1.c1"}}, {cohort: "outside", skip: "no_outside_family"}}},
		{"family collapse", [][]string{{"2.1.c1", "2.1.c2"}}, []string{"2.1"}, []filterSelection{{cohort: "inside", filter: []string{"2.1.c1", "2.1.c2"}}, {cohort: "outside", skip: "no_outside_family"}}},
		{"over 20 nodes collapses", [][]string{sameFamily}, []string{"2.1", "3.1"}, []filterSelection{{cohort: "inside", filter: []string{"2.1"}}, {cohort: "outside", filter: []string{"3.1"}}}},
		{"too many families", [][]string{many}, []string{"30.1"}, []filterSelection{{cohort: "inside", skip: "too_many_families"}, {cohort: "outside", filter: []string{"30.1"}}}},
		{"outside only", [][]string{nil}, []string{"2.1"}, []filterSelection{{cohort: "outside", filter: []string{"2.1"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := deriveFilterCases(tc.nodes, tc.families)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v want %+v", got, tc.want)
			}
		})
	}
}

func TestFilterGate_FailureAndSkippedCases(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: 11, Day: 9}
	for _, tc := range []struct {
		name     string
		anchor   string
		nodes    []string
		families []retrieval.NodeEntry
		status   string
		cohort   string
		skip     string
	}{
		{"one failing case", "missing", []string{"1.1.c1"}, []retrieval.NodeEntry{{Code: "1", Children: []retrieval.NodeEntry{{Code: "1.1"}, {Code: "1.2"}}}}, "fail", "inside", ""},
		{"no outside family", "one", []string{"1.1.c1"}, []retrieval.NodeEntry{{Code: "1", Children: []retrieval.NodeEntry{{Code: "1.1"}}}}, "pass", "outside", "no_outside_family"},
		{"too many families", "one", func() []string {
			var out []string
			for i := 1; i <= 21; i++ {
				out = append(out, fmt.Sprintf("%d.1.c1", i))
			}
			return out
		}(), []retrieval.NodeEntry{{Code: "1", Children: []retrieval.NodeEntry{{Code: "1.1"}}}}, "unavailable", "inside", "too_many_families"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			suite := recordedSuite()
			suite.Map.Entries[0].Alternatives = []Alternative{{Country: "vn", DocumentKey: "doc", Kind: "provision", Anchor: &tc.anchor}}
			engine := &filterFixtureEngine{recordedEngine: recordedEngine{fakeEngine: fakeEngine{date: date}}, nodes: map[retrieval.AnchorRef][]string{{DocumentKey: "doc", Anchor: tc.anchor}: tc.nodes}, families: tc.families}
			r, _, err := Run(context.Background(), engine, suite, Options{EvaluationDate: date})
			if err != nil {
				t.Fatal(err)
			}
			for _, gate := range r.FilterGate.Modes {
				if gate.Status != tc.status {
					t.Fatalf("status %s want %s", gate.Status, tc.status)
				}
			}
			if tc.status != "pass" && r.QualityPass {
				t.Fatal("failed gate passed quality")
			}
			cohort := r.FilterGate.Cohorts["exact"][tc.cohort]
			if tc.skip != "" && cohort.Skipped[tc.skip] != 1 {
				t.Fatalf("skips %v", cohort.Skipped)
			}
			if len(r.FilterGate.Cases) != 4 {
				t.Fatalf("cases %d", len(r.FilterGate.Cases))
			}
		})
	}
}

func TestFilterGate_PartialAndDocumentAlternatives(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: 11, Day: 9}
	anchor := "one"
	suite := recordedSuite()
	suite.Legacy[0].ExpectedCitations = append(suite.Legacy[0].ExpectedCitations, Expectation{DocNumber: "doc"})
	suite.Map.Entries[0].Alternatives = []Alternative{{Country: "vn", DocumentKey: "doc", Kind: "provision", Anchor: &anchor}}
	suite.Map.Entries = append(suite.Map.Entries, MapEntry{CaseID: "positive", CitationIndex: 1, State: "resolved", Alternatives: []Alternative{{Country: "vn", DocumentKey: "doc", Kind: "document"}}})
	engine := &filterFixtureEngine{recordedEngine: recordedEngine{fakeEngine: fakeEngine{date: date}}, nodes: map[retrieval.AnchorRef][]string{{DocumentKey: "doc", Anchor: "one"}: {"1.1.c1"}}, families: []retrieval.NodeEntry{{Code: "1", Children: []retrieval.NodeEntry{{Code: "1.1"}, {Code: "1.2"}}}}}
	r, _, err := Run(context.Background(), engine, suite, Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	c := r.FilterGate.Cohorts["exact"]["partial"]
	if c.CaseCount != 1 || c.Passed != 1 {
		t.Fatalf("partial %+v", c)
	}
	for _, row := range r.FilterGate.Cases {
		if row.Cohort == "partial" && (row.FilteredRecall8 == nil || *row.FilteredRecall8 != 1 || row.Within100 == nil || !*row.Within100) {
			t.Fatalf("row %+v", row)
		}
	}
}

func TestFilterGate_PassRules(t *testing.T) {
	a, b := "a", "b"
	entries := map[int]MapEntry{0: {State: "resolved", Alternatives: []Alternative{{Kind: "provision", Country: "vn", DocumentKey: "doc", Anchor: &a}}}, 1: {State: "resolved", Alternatives: []Alternative{{Kind: "provision", Country: "vn", DocumentKey: "doc", Anchor: &b}}}}
	hitA := retrieval.Hit{Identity: retrieval.Identity{Country: "vn", DocumentKey: "doc", Anchor: "a"}}
	hitB := retrieval.Hit{Identity: retrieval.Identity{Country: "vn", DocumentKey: "doc", Anchor: "b"}}
	for _, tc := range []struct {
		name, cohort         string
		seeded               []bool
		first, baseline, all []retrieval.Hit
		passed               bool
	}{
		{"inside recall falls", "inside", []bool{true, true}, []retrieval.Hit{hitA}, []retrieval.Hit{hitA, hitB}, []retrieval.Hit{hitA, hitB}, false},
		{"inside missing within 100", "inside", []bool{true, true}, []retrieval.Hit{hitA}, []retrieval.Hit{hitA}, []retrieval.Hit{hitA}, false},
		{"partial seeded unchanged despite total fall", "partial", []bool{true, false}, []retrieval.Hit{hitA}, []retrieval.Hit{hitA, hitB}, []retrieval.Hit{hitA, hitB}, true},
		{"partial seeded falls despite total unchanged", "partial", []bool{true, false}, []retrieval.Hit{hitB}, []retrieval.Hit{hitA}, []retrieval.Hit{hitA, hitB}, false},
		{"outside ignores recall", "outside", []bool{true, true}, nil, []retrieval.Hit{hitA, hitB}, []retrieval.Hit{hitA, hitB}, true},
		{"outside missing within 100", "outside", []bool{true, true}, []retrieval.Hit{hitA}, []retrieval.Hit{hitA}, []retrieval.Hit{hitA}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := FilterCase{Cohort: tc.cohort}
			scoreFilterCase(&row, entries, tc.seeded, tc.first, tc.baseline, tc.all)
			if row.Passed != tc.passed {
				t.Fatalf("passed %v", row.Passed)
			}
		})
	}
}

func TestFilterGate_FirstHundredUsesPageOrder(t *testing.T) {
	var pages []retrieval.Response
	for page := 0; page < 13; page++ {
		var primary, other []retrieval.Hit
		for i := 0; i < 7; i++ {
			primary = append(primary, retrieval.Hit{Identity: retrieval.Identity{Anchor: fmt.Sprint(page*8 + i + 1)}})
		}
		other = append(other, retrieval.Hit{Identity: retrieval.Identity{Anchor: fmt.Sprint(page*8 + 8)}})
		pages = append(pages, retrieval.Response{Hits: primary, NonCurrent: other})
	}
	first, all := filterPageHits(pages)
	if len(first) != 8 || first[7].Identity.Anchor != "8" || len(all) != 100 || all[99].Identity.Anchor != "100" {
		t.Fatalf("first %d all %d", len(first), len(all))
	}
}

func TestFilterGate_DocumentOnlyHasNoInsideCase(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: 11, Day: 9}
	engine := &filterFixtureEngine{recordedEngine: recordedEngine{fakeEngine: fakeEngine{date: date}}, families: []retrieval.NodeEntry{{Code: "1", Children: []retrieval.NodeEntry{{Code: "1.1"}}}}}
	r, _, err := Run(context.Background(), engine, recordedSuite(), Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"exact", "ann"} {
		c := r.FilterGate.Cohorts[mode]
		if c["inside"].CaseCount != 0 || c["partial"].CaseCount != 0 || c["outside"].CaseCount != 1 {
			t.Fatalf("cohorts %s %+v", mode, c)
		}
	}
}

type truncatedFilterEngine struct{ fakeEngine }

func (*truncatedFilterEngine) Taxonomy(_ context.Context, req retrieval.TaxonomyRequest) (retrieval.TaxonomyResult, error) {
	if req.Depth == 2 {
		return retrieval.TaxonomyResult{Truncated: true}, nil
	}
	if req.Node == "" {
		return retrieval.TaxonomyResult{Nodes: []retrieval.NodeEntry{{Code: "1"}, {Code: "2"}}}, nil
	}
	if req.Node == "1" {
		return retrieval.TaxonomyResult{Nodes: []retrieval.NodeEntry{{Code: "1.1"}}}, nil
	}
	return retrieval.TaxonomyResult{Nodes: []retrieval.NodeEntry{{Code: "2.1"}}}, nil
}
func TestFilterGate_TruncatedTreeLoadsFamiliesByRoot(t *testing.T) {
	s, err := newFilterState(context.Background(), &truncatedFilterEngine{}, recordedSuite(), Options{})
	if err != nil || !reflect.DeepEqual(s.families, []string{"1.1", "2.1"}) {
		t.Fatalf("families %v: %v", s.families, err)
	}
}
