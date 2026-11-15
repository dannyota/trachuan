package eval

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/apps/corpus/internal/stage/build"
)

func TestInheritanceQuality_NoAssignmentsDoesNotBlock(t *testing.T) {
	r := evaluation.Report{QualityPass: true}
	applyInheritance(&r, build.TaxonomyStats{}, false)
	if !r.QualityPass {
		t.Fatal("no inherited assignments blocked quality")
	}
}

func TestInheritanceQuality_GatesServedAssignments(t *testing.T) {
	for _, tc := range []struct {
		status    string
		inherited int64
		want      bool
	}{{"pass", 1, true}, {"fail", 1, false}, {"unavailable", 1, false}, {"fail", 0, true}, {"unavailable", 0, true}} {
		stats := build.TaxonomyStats{Inherited: tc.inherited}
		stats.Inheritance.Status = tc.status
		r := evaluation.Report{QualityPass: true, NotRun: map[string]string{"taxonomy": "out_of_scope"}}
		applyInheritance(&r, stats, false)
		if r.QualityPass != tc.want || r.Inheritance == nil {
			t.Fatalf("status %s count %d: %+v", tc.status, tc.inherited, r)
		}
	}
}

func TestInheritanceQuality_NoTaxonomyIsOutOfScope(t *testing.T) {
	r := evaluation.Report{QualityPass: true, NotRun: map[string]string{"taxonomy": "out_of_scope"}}
	applyInheritance(&r, build.TaxonomyStats{}, false)
	if r.Inheritance != nil || r.NotRun["taxonomy"] != "out_of_scope" {
		t.Fatalf("unexpected inheritance report: %+v", r.Inheritance)
	}
}

func TestInheritanceQuality_DiagnosticDoesNotChangeOutOfScope(t *testing.T) {
	for _, status := range []string{"pass", "fail", "unavailable"} {
		t.Run(status, func(t *testing.T) {
			stats := build.TaxonomyStats{Seeds: 2, Overrides: 1}
			stats.Inheritance.Status = status
			r := evaluation.Report{QualityPass: true, NotRun: map[string]string{"taxonomy": "out_of_scope"}}
			applyInheritance(&r, stats, false)
			if r.NotRun["taxonomy"] != "out_of_scope" || r.Inheritance == nil || !r.QualityPass {
				t.Fatalf("diagnostic=%v taxonomy=%q quality=%t", r.Inheritance, r.NotRun["taxonomy"], r.QualityPass)
			}
		})
	}
}

func TestInheritanceQuality_LLMIsInScope(t *testing.T) {
	stats := build.TaxonomyStats{}
	stats.Inheritance.Status = "unavailable"
	r := evaluation.Report{QualityPass: true, NotRun: map[string]string{"taxonomy": "out_of_scope"}}
	applyInheritance(&r, stats, true)
	if _, ok := r.NotRun["taxonomy"]; ok || r.Inheritance == nil {
		t.Fatalf("taxonomy=%v diagnostic=%v", r.NotRun, r.Inheritance)
	}
}

func TestInheritanceQuality_LLMWithoutDiagnosticIsInScope(t *testing.T) {
	r := evaluation.Report{QualityPass: true, NotRun: map[string]string{"taxonomy": "out_of_scope"}}
	applyInheritance(&r, build.TaxonomyStats{}, true)
	if _, ok := r.NotRun["taxonomy"]; ok {
		t.Fatal("LLM assignments marked out of scope")
	}
}
