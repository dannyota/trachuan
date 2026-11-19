package evaluation

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

func toleranceReport(t *testing.T, value float64) Report {
	t.Helper()
	r := Report{Version: 3, EvaluationDate: retrieval.Date{Year: 2099, Month: 1, Day: 2}, Scorer: "agent-eval/6", Recipes: map[string]string{"query": "q", "document": "d", "chunk": "c", "lexical": "l", "retrieval": "r"}, FixtureHashes: FixtureHashes{Legacy: "a", OldProduct: "b", Development: "c"}, Cohorts: map[string]Cohort{}, Rejections: []Rejection{{CaseID: "r1", Passed: true}, {CaseID: "r2", Passed: true}, {CaseID: "r3", Passed: true}}}
	for _, name := range []string{"old_product", "development"} {
		c := Cohort{CaseCount: 1, PositiveCount: 1, ExpectedCitations: 1}
		if name == "old_product" {
			c.CaseCount = 4
			c.RejectionCount = 3
		}
		for _, mode := range []retrieval.Mode{"exact", "ann"} {
			for _, spec := range []struct {
				name  string
				limit int
				value float64
			}{{"current_law_precision", 8, 1}, {"recall", 20, .95}, {"mrr", 8, value}, {"abstention_balanced_accuracy", 8, .85}} {
				v := spec.value
				c.Metrics = append(c.Metrics, Metric{Mode: mode, Arms: "hybrid", Limit: spec.limit, Name: spec.name, Value: &v, Denominator: 1})
			}
			for _, limit := range []int{8, 20} {
				r.Cases = append(r.Cases, CaseResult{CaseID: "one", Cohort: name, Mode: mode, Arms: "hybrid", Limit: limit, Expected: 1, Ranks: []int{4}, Mapped: [][]Alternative{{{Country: "vn", DocumentKey: "doc", Kind: "document"}}}})
			}
		}
		r.Cohorts[name] = c
	}
	return withComparisonRecipe(t, r, retrieval.Recipe3())
}

func mrrRecord(t *testing.T, r Report, cohort string) map[string]any {
	t.Helper()
	b, err := json.Marshal(r.Cohorts[cohort].NonRegression)
	if err != nil {
		t.Fatal(err)
	}
	var entries []map[string]any
	if err = json.Unmarshal(b, &entries); err != nil {
		t.Fatal(err)
	}
	for _, n := range entries {
		if n["name"] == "mrr" && n["mode"] == "exact" {
			return n
		}
	}
	t.Fatal("missing MRR comparison")
	return nil
}

func TestMRRTolerance_Conditions(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Report, *Report)
		pass   bool
	}{
		{"same recipes", func(*Report, *Report) {}, true},
		{"recipe differs", func(r, p *Report) { r.Recipes["chunk"] = "changed" }, false},
		{"scorer incompatible", func(r, p *Report) { p.Scorer = "agent-eval/3" }, false},
		{"compatible scorer", func(r, p *Report) { p.Scorer = "agent-eval/5"; p.Version = 2 }, true},
		{"agent-eval/4 incompatible", func(r, p *Report) { p.Scorer = "agent-eval/4"; p.Version = 1 }, false},
		{"legacy fixture differs", func(r, p *Report) { r.FixtureHashes.Legacy = "changed" }, false},
		{"old product fixture differs", func(r, p *Report) { r.FixtureHashes.OldProduct = "changed" }, false},
		{"case set differs", func(r, p *Report) {
			for i := range r.Cases {
				if r.Cases[i].Cohort == "old_product" {
					r.Cases[i].CaseID = "other"
				}
			}
		}, false},
		{"denominator differs", func(r, p *Report) {
			c := r.Cohorts["old_product"]
			c.Metrics[2].Denominator = 2
			r.Cohorts["old_product"] = c
		}, false},
		{"rejections differ", func(r, p *Report) { r.Rejections[0].CaseID = "other" }, false},
		{"expectation removed", func(r, p *Report) { r.Cases[0].Mapped = nil }, false},
		{"previous alternatives absent", func(r, p *Report) { p.Cases[0].Mapped = nil }, false},
		{"provenance changes", func(r, p *Report) { r.Cases[0].Mapped[0][0].DocumentNumber = "new" }, true},
		{"version one strict", func(r, p *Report) { r.Version = 1; p.Version = 1 }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, r := toleranceReport(t, .7247), toleranceReport(t, .7226)
			r.Cases[0].Ranks = []int{9}
			tc.mutate(&r, &p)
			if err := finishQuality(&r, &p); err != nil {
				t.Fatal(err)
			}
			n := mrrRecord(t, r, "old_product")
			if (n["status"] == "pass") != tc.pass {
				t.Fatalf("comparison: %v", n)
			}
			if tc.pass && (n["previous"] != .7247 || n["high_water"] != .7247 || n["tolerance"] != .005 || n["within_tolerance"] != true) {
				t.Fatalf("tolerance fields: %v", n)
			}
		})
	}
}

func TestMRRTolerance_WatermarkChainAndBoundary(t *testing.T) {
	p := toleranceReport(t, .7247)
	for _, value := range []float64{.7226, .7200, .7197, math.Nextafter(.7197, 0)} {
		r := toleranceReport(t, value)
		if err := finishQuality(&r, &p); err != nil {
			t.Fatal(err)
		}
		n := mrrRecord(t, r, "old_product")
		if n["high_water"] != .7247 || (n["status"] == "pass") != (value >= .7247-.005) {
			t.Fatalf("value %v: %v", value, n)
		}
		p = r
	}
	r := toleranceReport(t, .72)
	recipe := retrieval.Recipe3()
	recipe.DocumentCap = 6
	r = withComparisonRecipe(t, r, recipe)
	if err := finishQuality(&r, &p); err != nil {
		t.Fatal(err)
	}
	if n := mrrRecord(t, r, "old_product"); n["high_water"] != *p.Cohorts["old_product"].Metrics[2].Value {
		t.Fatalf("recipe change retained watermark: %v", n)
	}
}

func TestMRRTolerance_ReleaseNumbers(t *testing.T) {
	p, r := toleranceReport(t, .7247), toleranceReport(t, .7226)
	for _, name := range []string{"old_product", "development"} {
		pc, rc := p.Cohorts[name], r.Cohorts[name]
		for i := range pc.Metrics {
			switch pc.Metrics[i].Name {
			case "recall":
				pc.Metrics[i].Value = new(.9247)
				rc.Metrics[i].Value = new(.9570)
			case "mrr":
				if name == "development" {
					pc.Metrics[i].Value = new(.6616)
					rc.Metrics[i].Value = new(.6706)
				} else if pc.Metrics[i].Mode == "ann" {
					pc.Metrics[i].Value = new(.7241)
				}
			}
		}
		p.Cohorts[name], r.Cohorts[name] = pc, rc
	}
	for i := range r.Cases {
		if r.Cases[i].Cohort == "old_product" {
			r.Cases[i].Ranks = []int{9}
		}
	}
	if err := finishQuality(&r, &p); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.NonRegression, r.Cohorts["old_product"].NonRegression) {
		t.Fatal("top-level comparison differs")
	}
	for _, name := range []string{"old_product", "development"} {
		for _, record := range r.Cohorts[name].NonRegression {
			if record.Name != "mrr" {
				continue
			}
			previous := .6616
			if name == "old_product" {
				previous = .7247
				if record.Mode == "ann" {
					previous = .7241
				}
			}
			if record.Status != "pass" || record.Previous == nil || *record.Previous != previous || record.WithinTolerance != (name == "old_product") {
				t.Fatalf("%s: %+v", name, record)
			}
		}
	}
	if !r.QualityPass {
		t.Fatal("release numbers failed quality")
	}
	n := mrrRecord(t, r, "old_product")
	losses, ok := n["losses"].([]any)
	if !ok || len(losses) != 1 {
		t.Fatalf("losses: %v", n)
	}
	loss := losses[0].(map[string]any)
	if loss["previous_rank"] != float64(4) || loss["rank"] != float64(9) {
		t.Fatalf("loss: %v", loss)
	}
	if mrrRecord(t, r, "development")["within_tolerance"] != false {
		t.Fatal("development improvement marked tolerated")
	}
}

func TestMRRTolerance_StrictFallbackAndVersionOne(t *testing.T) {
	p := toleranceReport(t, .7247)
	middle := toleranceReport(t, .7226)
	if err := finishQuality(&middle, &p); err != nil {
		t.Fatal(err)
	}
	r := toleranceReport(t, .723)
	r.FixtureHashes.Legacy = "changed"
	if err := finishQuality(&r, &middle); err != nil {
		t.Fatal(err)
	}
	n := mrrRecord(t, r, "old_product")
	if n["status"] != "pass" || n["tolerance"] != float64(0) || n["high_water"] != .7247 {
		t.Fatalf("strict fallback: %v", n)
	}
	r = toleranceReport(t, .722)
	r.Version = 1
	if err := finishQuality(&r, &middle); err == nil || r.QualityPass {
		t.Fatal("version downgrade accepted")
	}

}

func TestMRRTolerance_RecallRemainsStrict(t *testing.T) {
	p, r := toleranceReport(t, .7247), toleranceReport(t, .7226)
	c := r.Cohorts["development"]
	c.Metrics[1].Value = new(.9499)
	r.Cohorts["development"] = c
	if err := finishQuality(&r, &p); err != nil {
		t.Fatal(err)
	}
	if r.QualityPass {
		t.Fatal("recall regression passed")
	}
}

func TestMRRTolerance_CandidateNeverEntersWatermark(t *testing.T) {
	p, r := toleranceReport(t, .72), toleranceReport(t, .75)
	if err := finishQuality(&r, &p); err != nil {
		t.Fatal(err)
	}
	if n := mrrRecord(t, r, "old_product"); n["high_water"] != .72 {
		t.Fatalf("candidate entered watermark: %v", n)
	}
	next := toleranceReport(t, .746)
	if err := finishQuality(&next, &r); err != nil {
		t.Fatal(err)
	}
	if n := mrrRecord(t, next, "old_product"); n["high_water"] != .75 || n["status"] != "pass" {
		t.Fatalf("gain not retained: %v", n)
	}
}

func TestPreviousReport_MissingWatermarkRefused(t *testing.T) {
	p, r := toleranceReport(t, .7247), toleranceReport(t, .7226)
	if err := finishQuality(&r, &p); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err = json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	cohorts := raw["cohorts"].(map[string]any)
	records := cohorts["old_product"].(map[string]any)["non_regression"].([]any)
	for _, entry := range records {
		row := entry.(map[string]any)
		if row["name"] == "mrr" {
			delete(row, "high_water")
		}
	}
	data, err = json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = decodePreviousReport(data); err == nil {
		t.Fatal("previous version two report without watermark accepted")
	}
}

func TestMRRTolerance_LossRanksAcrossLimits(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		rank20, expected, want int
	}{{"outside eight", 9, 1, 9}, {"different shaping", 4, 1, 0}, {"removed expectation", 0, 0, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			p, r := toleranceReport(t, .7247), toleranceReport(t, .7226)
			r.Cases[0].Ranks = []int{0}
			r.Cases[0].Expected = tc.expected
			if tc.expected == 0 {
				r.Cases[0].Ranks = nil
				r.Cases[0].Mapped = nil
			}
			r.Cases[1].Ranks = []int{tc.rank20}
			if err := finishQuality(&r, &p); err != nil {
				t.Fatal(err)
			}
			n := mrrRecord(t, r, "old_product")
			losses := n["losses"].([]any)
			if len(losses) != 1 || losses[0].(map[string]any)["rank"] != float64(tc.want) {
				t.Fatalf("losses: %v", losses)
			}
		})
	}
}

func TestMRRTolerance_ConditionThreePerCohort(t *testing.T) {
	p, r := toleranceReport(t, .7247), toleranceReport(t, .7226)
	r.Cases[0].Ranks = []int{9}
	r.FixtureHashes.Legacy = "changed"
	if err := finishQuality(&r, &p); err != nil {
		t.Fatal(err)
	}
	if n := mrrRecord(t, r, "old_product"); n["tolerance"] != float64(0) || n["status"] != "fail" {
		t.Fatalf("old product with a changed legacy fixture: %v", n)
	}
	if n := mrrRecord(t, r, "development"); n["tolerance"] != mrrTolerance || n["status"] != "pass" {
		t.Fatalf("development with a changed legacy fixture: %v", n)
	}
	p, r = toleranceReport(t, .7247), toleranceReport(t, .7226)
	r.Cases[0].Ranks = []int{9}
	r.FixtureHashes.Development = "changed"
	r.revision = &developmentRevision{comparable: []string{"one"}, previousOrder: []string{"one"}, revised: []RevisedCase{}}
	if err := finishQuality(&r, &p); err != nil {
		t.Fatal(err)
	}
	if n := mrrRecord(t, r, "old_product"); n["tolerance"] != mrrTolerance || n["status"] != "pass" {
		t.Fatalf("old product with a changed development fixture: %v", n)
	}
	if n := mrrRecord(t, r, "development"); n["revision"] == nil || n["tolerance"] != mrrTolerance {
		t.Fatalf("development revision record: %v", n)
	}
}
