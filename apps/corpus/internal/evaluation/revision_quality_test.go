package evaluation

import (
	"math"
	"reflect"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

type revisionCase struct {
	id       string
	r8, r20  []int
	cohort   string
	question string
}

func setDevelopmentRecords(t *testing.T, r *Report, cases []revisionCase) []Case {
	t.Helper()
	var kept []CaseResult
	for _, c := range r.Cases {
		if c.Cohort != "development" && c.Cohort != "development_period" {
			kept = append(kept, c)
		}
	}
	counts := map[string]int{}
	var fixture []Case
	for _, c := range cases {
		cohort := c.cohort
		if cohort == "" {
			cohort = "development"
		}
		counts[cohort]++
		question := c.question
		if question == "" {
			question = "q-" + c.id
		}
		f := fixtureCase(c.id, question, len(c.r8))
		if cohort == "development_period" {
			f.Tags = []string{"historical"}
		}
		fixture = append(fixture, f)
		for _, mode := range []retrieval.Mode{"exact", "ann"} {
			for _, limit := range []int{8, 20} {
				ranks := c.r8
				if limit == 20 {
					ranks = c.r20
				}
				mapped := make([][]Alternative, len(ranks))
				for i := range mapped {
					mapped[i] = []Alternative{{Country: "vn", DocumentKey: "doc", Kind: "document"}}
				}
				kept = append(kept, CaseResult{CaseID: c.id, Cohort: cohort, Mode: mode, Arms: "hybrid", Limit: limit, Expected: len(ranks), Ranks: append([]int(nil), ranks...), Mapped: mapped})
			}
		}
	}
	r.Cases = kept
	development := r.Cohorts["development"]
	development.CaseCount = counts["development"]
	r.Cohorts["development"] = development
	if counts["development_period"] > 0 {
		r.Cohorts["development_period"] = Cohort{CaseCount: counts["development_period"]}
	}
	return fixture
}

func revisionPair(t *testing.T, previous, current []revisionCase) (Report, Report) {
	t.Helper()
	p, r := toleranceReport(t, .5), toleranceReport(t, .5)
	p.Version, p.Scorer = 2, "agent-eval/5"
	p.FixtureHashes.Development = "previous-development"
	previousFixture := setDevelopmentRecords(t, &p, previous)
	currentFixture := setDevelopmentRecords(t, &r, current)
	cohorts, err := previousCohorts(&p)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := compareFixtures(previousFixture, currentFixture, cohorts)
	if err != nil {
		t.Fatal(err)
	}
	r.revision = &revision
	return p, r
}

func developmentRecord(t *testing.T, r Report, mode retrieval.Mode, name string, limit int) NonRegression {
	t.Helper()
	for _, n := range r.Cohorts["development"].NonRegression {
		if n.Mode == mode && n.Name == name && n.Limit == limit {
			return n
		}
	}
	t.Fatalf("missing development %s %s@%d", mode, name, limit)
	return NonRegression{}
}

func near(v *float64, want float64) bool { return v != nil && math.Abs(*v-want) < 1e-12 }

func TestRevision_RecomputedValuesInFixtureOrder(t *testing.T) {
	previous := []revisionCase{
		{id: "p1", r8: []int{2}, r20: []int{2}},
		{id: "p2", r8: []int{0, 3}, r20: []int{0, 3}},
		{id: "p3", r8: []int{5}, r20: []int{5}},
		{id: "m", r8: []int{1}, r20: []int{1}},
		{id: "n"},
	}
	current := []revisionCase{
		{id: "p1", r8: []int{1}, r20: []int{1}},
		{id: "p2", r8: []int{0, 4}, r20: []int{12, 4}},
		{id: "p3", r8: []int{6}, r20: []int{6}},
		{id: "m", r8: []int{0}, r20: []int{0}, cohort: "development_period"},
		{id: "n"},
	}
	p, r := revisionPair(t, previous, current)
	if err := finishQuality(&r, &p); err != nil {
		t.Fatal(err)
	}
	mrr := developmentRecord(t, r, "exact", "mrr", 8)
	previousMRR := (1.0/2 + 1.0/3 + 1.0/5) / 3
	candidateMRR := (1 + 1.0/4 + 1.0/6) / 3
	if !near(mrr.PreviousValue, previousMRR) || !near(mrr.Previous, previousMRR) || mrr.Revision == nil || !near(mrr.Revision.Value, candidateMRR) {
		t.Fatalf("mrr = %+v revision %+v", mrr, mrr.Revision)
	}
	if mrr.Value == nil || *mrr.Value != .5 {
		t.Fatalf("value over all cases changed: %v", mrr.Value)
	}
	if mrr.Revision.Cases != 4 || !reflect.DeepEqual(mrr.Revision.Revised, []RevisedCase{{"m", "cohort"}}) || mrr.Numerator != nil {
		t.Fatalf("revision = %+v", mrr.Revision)
	}
	if mrr.Status != "pass" || mrr.WithinTolerance || !reflect.DeepEqual(mrr.Losses, []MRRLoss{{"p2", 3, 4}, {"p3", 5, 6}}) {
		t.Fatalf("mrr status = %+v", mrr)
	}
	recall := developmentRecord(t, r, "exact", "recall", 20)
	rv := recall.Revision
	if rv == nil || recall.Numerator == nil || *recall.Numerator != 4 || *recall.Denominator != 4 || *recall.PreviousNumerator != 3 || *recall.PreviousDenominator != 4 {
		t.Fatalf("recall = %+v revision %+v", recall, rv)
	}
	if !near(recall.PreviousValue, .75) || !near(rv.Value, 1) || recall.Status != "pass" || recall.MRRComparison != nil {
		t.Fatalf("recall = %+v", recall)
	}
	for _, n := range r.Cohorts["development"].NonRegression {
		if n.Name == "abstention_balanced_accuracy" && n.Revision != nil {
			t.Fatal("abstention record carries a revision")
		}
	}
	for _, n := range r.Cohorts["old_product"].NonRegression {
		if n.Revision != nil {
			t.Fatal("old product record carries a revision")
		}
	}
}

func TestRevision_RecallComparesIntegerCounts(t *testing.T) {
	previous := []revisionCase{
		{id: "a", r8: []int{1, 2}, r20: []int{1, 2}},
		{id: "b", r8: []int{0}, r20: []int{9}},
		{id: "x", r8: []int{1}, r20: []int{1}},
	}
	for _, tc := range []struct {
		name   string
		b      []int
		status string
		count  int
	}{{"same count", []int{3}, "pass", 3}, {"one fewer", []int{0}, "fail", 2}} {
		t.Run(tc.name, func(t *testing.T) {
			current := []revisionCase{
				{id: "a", r8: []int{1, 2}, r20: []int{1, 2}},
				{id: "b", r8: tc.b, r20: tc.b},
				{id: "x", r8: []int{1}, r20: []int{1}, question: "changed"},
			}
			p, r := revisionPair(t, previous, current)
			if err := finishQuality(&r, &p); err != nil {
				t.Fatal(err)
			}
			for _, mode := range []retrieval.Mode{"exact", "ann"} {
				n := developmentRecord(t, r, mode, "recall", 20)
				rv := n.Revision
				if n.Status != tc.status || *n.Numerator != tc.count || *n.PreviousNumerator != 3 || *n.Denominator != 3 || *n.PreviousDenominator != 3 || rv.Cases != 2 {
					t.Fatalf("%s: %+v revision %+v", mode, n, rv)
				}
				if !reflect.DeepEqual(rv.Revised, []RevisedCase{{"x", "changed"}}) {
					t.Fatalf("revised = %v", rv.Revised)
				}
			}
			if r.QualityPass != (tc.status == "pass") {
				t.Fatalf("quality = %v", r.QualityPass)
			}
		})
	}
}

func setPreviousMRR(p *Report, value, highWater float64, revision *float64) {
	development := p.Cohorts["development"]
	development.NonRegression = nil
	for _, mode := range []retrieval.Mode{"exact", "ann"} {
		v, hw := value, highWater
		n := NonRegression{Mode: mode, Arms: "hybrid", Limit: 8, Name: "mrr", Value: &v, PreviousValue: &hw, Gating: true, Status: "pass", MRRComparison: &MRRComparison{Previous: &hw, HighWater: &hw, Tolerance: mrrTolerance, WithinTolerance: true, Losses: []MRRLoss{}}}
		if revision != nil {
			rv := *revision
			n.Revision = &Revision{Cases: 1, Value: &rv, Revised: []RevisedCase{}}
		}
		development.NonRegression = append(development.NonRegression, n)
	}
	p.Cohorts["development"] = development
}

func TestRevision_HighWaterCarriesToleranceUsed(t *testing.T) {
	previous := []revisionCase{
		{id: "a", r8: []int{7}, r20: []int{7}},
		{id: "b", r8: []int{1}, r20: []int{1}},
		{id: "c", r8: []int{1}, r20: []int{1}},
		{id: "d", r8: []int{1}, r20: []int{1}},
		{id: "m", r8: []int{1}, r20: []int{1}},
	}
	current := []revisionCase{
		{id: "a", r8: []int{8}, r20: []int{8}},
		{id: "b", r8: []int{1}, r20: []int{1}},
		{id: "c", r8: []int{1}, r20: []int{1}},
		{id: "d", r8: []int{1}, r20: []int{1}},
		{id: "m", r8: []int{1}, r20: []int{1}, cohort: "development_period"},
	}
	recomputed := (1.0/7 + 3) / 4
	candidate := (1.0/8 + 3) / 4
	for _, tc := range []struct {
		name      string
		value, hw float64
		revision  *float64
		used      float64
	}{
		{"plain previous", .7, .703, nil, .003},
		{"plain previous at watermark", .8, .8, nil, 0},
		{"revision previous", .71, .703, new(.7), .003},
		{"revision previous above watermark", .71, .703, new(.705), 0},
	} {
		wantStatus := "pass"
		if tc.used > 0 {
			wantStatus = "fail"
		}
		t.Run(tc.name, func(t *testing.T) {
			p, r := revisionPair(t, previous, current)
			setPreviousMRR(&p, tc.value, tc.hw, tc.revision)
			if err := finishQuality(&r, &p); err != nil {
				t.Fatal(err)
			}
			n := developmentRecord(t, r, "exact", "mrr", 8)
			if !near(n.HighWater, recomputed+tc.used) || !near(n.Previous, recomputed) || !near(n.Revision.Value, candidate) || n.Tolerance != mrrTolerance {
				t.Fatalf("record = %+v revision %+v", n.MRRComparison, n.Revision)
			}
			if n.Status != wantStatus || !n.WithinTolerance || len(n.Losses) != 1 || n.Losses[0] != (MRRLoss{"a", 7, 8}) {
				t.Fatalf("status %s want %s: %+v", n.Status, wantStatus, n.MRRComparison)
			}
		})
	}
	t.Run("recipe change drops the watermark", func(t *testing.T) {
		p, r := revisionPair(t, previous, current)
		setPreviousMRR(&p, .7, .703, nil)
		r.Recipes["chunk"] = "changed"
		if err := finishQuality(&r, &p); err != nil {
			t.Fatal(err)
		}
		n := developmentRecord(t, r, "exact", "mrr", 8)
		if !near(n.HighWater, recomputed) || n.Tolerance != 0 || n.Status != "fail" {
			t.Fatalf("record = %+v", n.MRRComparison)
		}
	})
}

func TestRevision_PlainRecordAfterRevisionRecord(t *testing.T) {
	p, r := toleranceReport(t, .7), toleranceReport(t, .69)
	setPreviousMRR(&p, .7, .703, new(.69))
	if err := finishQuality(&r, &p); err != nil {
		t.Fatal(err)
	}
	n := developmentRecord(t, r, "exact", "mrr", 8)
	if !near(n.HighWater, .7+(.703-.69)) || n.Revision != nil || n.Status != "fail" {
		t.Fatalf("record = %+v", n.MRRComparison)
	}
}

func TestRevision_UnresolvedRevisionRefused(t *testing.T) {
	p, r := toleranceReport(t, .7), toleranceReport(t, .7)
	r.FixtureHashes.Development = "revised"
	if err := finishQuality(&r, &p); err == nil || r.QualityPass {
		t.Fatal("revised fixture compared without a revision")
	}
}
