package evaluation

import (
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

func developmentCaseRecord(t *testing.T, r *Report, id string, mode retrieval.Mode, limit int) *CaseResult {
	t.Helper()
	for i, c := range r.Cases {
		if c.CaseID == id && c.Cohort == "development" && c.Mode == mode && c.Arms == "hybrid" && c.Limit == limit {
			return &r.Cases[i]
		}
	}
	t.Fatalf("missing development record %s %s@%d", id, mode, limit)
	return nil
}

func withoutDevelopmentRecord(r *Report, id string, mode retrieval.Mode, limit int) {
	var kept []CaseResult
	for _, c := range r.Cases {
		if c.CaseID == id && c.Cohort == "development" && c.Mode == mode && c.Arms == "hybrid" && c.Limit == limit {
			continue
		}
		kept = append(kept, c)
	}
	r.Cases = kept
}

func clearDevelopmentMetrics(r *Report) {
	development := r.Cohorts["development"]
	development.Metrics = append([]Metric(nil), development.Metrics...)
	for i, m := range development.Metrics {
		if m.Name == "recall" || m.Name == "mrr" {
			development.Metrics[i].Value = nil
		}
	}
	r.Cohorts["development"] = development
}

func gatedDevelopmentRecords(t *testing.T, r Report) []NonRegression {
	t.Helper()
	var out []NonRegression
	for _, mode := range []retrieval.Mode{"exact", "ann"} {
		out = append(out, developmentRecord(t, r, mode, "recall", 20), developmentRecord(t, r, mode, "mrr", 8))
	}
	return out
}

func TestRevision_DuplicateRecordMakesValuesUnavailable(t *testing.T) {
	previous := []revisionCase{
		{id: "a", r8: []int{1}, r20: []int{1}},
		{id: "b", r8: []int{1}, r20: []int{1}},
		{id: "x", r8: []int{1}, r20: []int{1}},
	}
	current := []revisionCase{
		{id: "a", r8: []int{0}, r20: []int{0}},
		{id: "b", r8: []int{1}, r20: []int{1}},
		{id: "x", r8: []int{1}, r20: []int{1}, question: "changed"},
	}
	for _, tc := range []struct {
		name      string
		duplicate func(t *testing.T, p, r *Report)
		status    string
	}{
		{"intact reports", func(*testing.T, *Report, *Report) {}, "fail"},
		{"duplicated previous record", func(t *testing.T, p, _ *Report) {
			p.Cases = append(p.Cases, *developmentCaseRecord(t, p, "b", "exact", 20))
		}, "unavailable"},
		{"duplicated candidate record", func(t *testing.T, _, r *Report) {
			r.Cases = append(r.Cases, *developmentCaseRecord(t, r, "b", "ann", 8))
		}, "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, r := revisionPair(t, previous, current)
			tc.duplicate(t, &p, &r)
			if err := finishQuality(&r, &p); err != nil {
				t.Fatal(err)
			}
			for _, n := range gatedDevelopmentRecords(t, r) {
				if n.Status != tc.status || n.Revision == nil {
					t.Fatalf("%s %s@%d: status %s want %s", n.Mode, n.Name, n.Limit, n.Status, tc.status)
				}
				if tc.name == "duplicated previous record" && n.PreviousValue != nil {
					t.Fatalf("%s %s@%d: previous value %v from a duplicated record", n.Mode, n.Name, n.Limit, *n.PreviousValue)
				}
			}
			if r.QualityPass {
				t.Fatal("lost rank-1 hit passed quality")
			}
		})
	}
}

func TestRevision_MissingPreviousRecordMakesMRRUnavailable(t *testing.T) {
	previous := []revisionCase{
		{id: "a", r8: []int{1}, r20: []int{1}},
		{id: "b", r8: []int{1}, r20: []int{1}},
		{id: "x", r8: []int{1}, r20: []int{1}},
	}
	current := []revisionCase{
		{id: "a", r8: []int{2}, r20: []int{2}},
		{id: "b", r8: []int{1}, r20: []int{1}},
		{id: "x", r8: []int{1}, r20: []int{1}, question: "changed"},
	}
	p, r := revisionPair(t, previous, current)
	developmentCaseRecord(t, &r, "a", "ann", 8).Ranks = []int{1}
	withoutDevelopmentRecord(&p, "a", "exact", 8)
	if err := finishQuality(&r, &p); err != nil {
		t.Fatal(err)
	}
	if n := developmentRecord(t, r, "exact", "mrr", 8); n.Status != "unavailable" || n.PreviousValue != nil {
		t.Fatalf("exact MRR = %+v", n)
	}
	for _, n := range []NonRegression{developmentRecord(t, r, "ann", "mrr", 8), developmentRecord(t, r, "exact", "recall", 20), developmentRecord(t, r, "ann", "recall", 20)} {
		if n.Status != "pass" {
			t.Fatalf("%s %s@%d = %s", n.Mode, n.Name, n.Limit, n.Status)
		}
	}
	if r.QualityPass {
		t.Fatal("missing previous record passed quality")
	}
}

func TestRevision_MissingValueStatus(t *testing.T) {
	previous := []revisionCase{
		{id: "a", r8: []int{1}, r20: []int{1}},
		{id: "b", r8: []int{2}, r20: []int{2}},
		{id: "x", r8: []int{1}, r20: []int{1}},
	}
	current := []revisionCase{
		{id: "a", r8: []int{1}, r20: []int{1}},
		{id: "b", r8: []int{2}, r20: []int{2}},
		{id: "x", r8: []int{1}, r20: []int{1}, question: "changed"},
	}
	issue := []MappingIssue{{CitationIndex: 0, Kind: "missing", Field: "document_key", Detail: "absent"}}
	withIssues := func(t *testing.T, r *Report) {
		for _, mode := range []retrieval.Mode{"exact", "ann"} {
			for _, limit := range []int{8, 20} {
				developmentCaseRecord(t, r, "a", mode, limit).MappingIssues = issue
			}
		}
	}
	for _, tc := range []struct {
		name    string
		mutate  func(t *testing.T, p, r *Report)
		status  string
		quality bool
	}{
		{"values present", func(*testing.T, *Report, *Report) {}, "pass", true},
		{"whole-cohort value missing", func(_ *testing.T, _, r *Report) { clearDevelopmentMetrics(r) }, "unavailable", false},
		{"candidate record has mapping issues", func(t *testing.T, _, r *Report) { withIssues(t, r) }, "unavailable", false},
		{"previous record has mapping issues", func(t *testing.T, p, _ *Report) { withIssues(t, p) }, "unavailable", false},
		{"no stated previous value", func(t *testing.T, p, _ *Report) {
			clearDevelopmentMetrics(p)
			withIssues(t, p)
		}, "no_previous", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, r := revisionPair(t, previous, current)
			tc.mutate(t, &p, &r)
			if err := finishQuality(&r, &p); err != nil {
				t.Fatal(err)
			}
			for _, n := range gatedDevelopmentRecords(t, r) {
				if n.Status != tc.status {
					t.Fatalf("%s %s@%d: status %s want %s", n.Mode, n.Name, n.Limit, n.Status, tc.status)
				}
			}
			if r.QualityPass != tc.quality {
				t.Fatalf("quality = %v", r.QualityPass)
			}
		})
	}
}
