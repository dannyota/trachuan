package evaluation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

func TestQuality_CohortCountsFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Report, *Report)
	}{
		{"forged development count", func(r, p *Report) { c := r.Cohorts["development"]; c.CaseCount = 0; r.Cohorts["development"] = c }},
		{"forged old product count", func(r, p *Report) { c := r.Cohorts["old_product"]; c.CaseCount = 0; r.Cohorts["old_product"] = c }},
		{"emptied development", func(r, p *Report) { dropDevelopment(r); r.Cohorts["development"] = Cohort{} }},
		{"absent development", func(r, p *Report) { dropDevelopment(r); delete(r.Cohorts, "development") }},
		{"forged previous count", func(r, p *Report) { c := p.Cohorts["development"]; c.CaseCount = 0; p.Cohorts["development"] = c }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, r := toleranceReport(t, .8), toleranceReport(t, .8)
			tc.mutate(&r, &p)
			if err := finishQuality(&r, &p); err == nil {
				t.Fatal("invalid report transition accepted")
			}
			if r.QualityPass {
				t.Fatal("invalid cohort counts passed quality")
			}
		})
	}
}

func dropDevelopment(r *Report) {
	var remaining []CaseResult
	for _, c := range r.Cases {
		if c.Cohort != "development" {
			remaining = append(remaining, c)
		}
	}
	r.Cases = remaining
}

func TestQuality_ReportVersionDowngradeRefused(t *testing.T) {
	p, r := toleranceReport(t, .796), toleranceReport(t, .796)
	r.Version = 1
	r.Scorer = "agent-eval/4"
	if err := finishQuality(&r, &p); err == nil {
		t.Fatal("report downgrade accepted")
	}
	if r.QualityPass {
		t.Fatal("version one candidate after version two passed")
	}
}

func TestQuality_VersionTwoChainKeepsWatermark(t *testing.T) {
	first := toleranceReport(t, .8)
	middle := toleranceReport(t, .796)
	if err := finishQuality(&middle, &first); err != nil {
		t.Fatal(err)
	}
	if !middle.QualityPass {
		t.Fatal("first tolerated loss failed")
	}
	last := toleranceReport(t, .792)
	if err := finishQuality(&last, &middle); err != nil {
		t.Fatal(err)
	}
	n := mrrRecord(t, last, "development")
	if last.QualityPass || n["status"] != "fail" || n["high_water"] != .8 {
		t.Fatalf("watermark reset: %v", n)
	}
}

func TestRun_EmptiedPreviousCohortReturnsNoReport(t *testing.T) {
	date := retrieval.Date{Year: 2099, Month: 1, Day: 2}
	suite := developmentSuite()
	previous, _, err := Run(context.Background(), &fakeEngine{date: date}, suite, Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(previous)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "previous.json")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	current, _, err := Run(context.Background(), &fakeEngine{date: date}, suite, Options{EvaluationDate: date, PreviousReport: path})
	if err != nil || current.Version != 3 {
		t.Fatalf("version three successor: version %d, error %v", current.Version, err)
	}
	suite.Development = nil
	current, _, err = Run(context.Background(), &fakeEngine{date: date}, suite, Options{EvaluationDate: date, PreviousReport: path})
	if err == nil || !strings.Contains(err.Error(), "populated previous cohort development") || current.Status != "" {
		t.Fatalf("report status %q, error %v", current.Status, err)
	}
}

func TestQuality_EmptyDevelopmentInBothReportsAllowed(t *testing.T) {
	previous, current := toleranceReport(t, .8), toleranceReport(t, .8)
	for _, r := range []*Report{&previous, &current} {
		dropDevelopment(r)
		r.Cohorts["development"] = Cohort{}
	}
	if err := finishQuality(&current, &previous); err != nil {
		t.Fatal(err)
	}
	if !current.QualityPass {
		t.Fatal("empty development changed product quality")
	}
}
