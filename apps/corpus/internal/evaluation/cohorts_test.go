package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

func TestDevelopment_ScoredSeparately(t *testing.T) {
	suite := developmentSuite()
	date := retrieval.Date{Year: 2026, Month: 11, Day: 9}
	r, cache, err := Run(context.Background(), &recordedEngine{fakeEngine: fakeEngine{date: date}}, suite, Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	if r.Cohorts["development"].CaseCount != 2 || r.Cohorts["development"].NegativeCount != 1 {
		t.Fatal("development was not scored separately")
	}
	if r.Cohorts["old_product"].CaseCount != 5 || r.Cohorts["legacy"].CaseCount != 5 {
		t.Fatal("development changed old cohort denominators")
	}
	development := r.Cohorts["development"]
	if development.ExpectedCitations != 1 || development.PositiveCount != 1 || len(development.Metrics) != len(r.Cohorts["old_product"].Metrics) {
		t.Fatal("development metric denominators differ")
	}
	for _, m := range development.Metrics {
		if m.Name == "recall" && (m.Numerator != 1 || m.Denominator != 1 || m.Value == nil || *m.Value != 1) {
			t.Fatalf("recall: %+v", m)
		}
	}
	rows := 0
	for _, index := range cache.Index {
		if index.Cohort == "development" {
			rows++
		}
	}
	if rows != 2 || len(cache.Vectors) != 5 {
		t.Fatal("development vectors not cached or reused")
	}

}

func developmentSuite() Suite {
	s := recordedSuite()
	s.Development = []Case{
		{ID: "development-positive", Question: "Câu hỏi tốt", ExpectedCitations: []Expectation{{DocNumber: "doc"}}, Tags: []string{"identifier"}},
		{ID: "development-negative", Question: "Câu hỏi không", ExpectAbstain: true, Tags: []string{"negative"}},
	}
	entry := s.Map.Entries[0]
	entry.CaseID = "development-positive"
	s.Map.Entries = append(s.Map.Entries, entry)
	s.Hashes.Development = "development-fixture"
	return s
}

type developmentSummary struct {
	Floors        []Floor         `json:"floors"`
	Targets       []Target        `json:"targets"`
	NonRegression []NonRegression `json:"non_regression"`
}

func readDevelopmentSummary(t *testing.T, r Report) developmentSummary {
	t.Helper()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Cohorts map[string]developmentSummary `json:"cohorts"`
	}
	if err = json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	summary, ok := decoded.Cohorts["development"]
	if !ok {
		t.Fatal("missing development cohort")
	}
	return summary
}

func TestDevelopment_NotRunWhenEmpty(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: 11, Day: 9}
	r, _, err := Run(context.Background(), &recordedEngine{fakeEngine: fakeEngine{date: date}}, recordedSuite(), Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	if !r.QualityPass || r.NotRun["development"] != "not_run" {
		t.Fatal("empty development changed quality or status")
	}
	summary := readDevelopmentSummary(t, r)
	if len(summary.Floors) != 2 {
		t.Fatal("missing development floors")
	}
	for _, f := range summary.Floors {
		if f.Status != "not_run" || f.Value != nil {
			t.Fatal("empty development floor was scored")
		}
	}
}

func TestDevelopment_FloorsApply(t *testing.T) {
	for _, pass := range []bool{true, false} {
		t.Run(fmt.Sprint(pass), func(t *testing.T) {
			s := developmentSuite()
			if !pass {
				s.Development[0].Question = "Câu hỏi hụt"
			}
			date := retrieval.Date{Year: 2026, Month: 11, Day: 9}
			r, _, err := Run(context.Background(), &recordedEngine{fakeEngine: fakeEngine{date: date}}, s, Options{EvaluationDate: date})
			if err != nil {
				t.Fatal(err)
			}
			if r.QualityPass != pass {
				t.Fatalf("quality=%v want %v", r.QualityPass, pass)
			}
			for _, f := range r.Floors {
				if f.Status != "pass" {
					t.Fatal("development changed old product floor")
				}
			}
			summary := readDevelopmentSummary(t, r)
			if len(summary.Floors) != 2 || len(summary.NonRegression) != 6 || len(summary.Targets) != 8 {
				t.Fatal("development quality records missing")
			}
			for _, f := range summary.Floors {
				if pass && f.Status != "pass" {
					t.Fatal("passing development floor failed")
				}
				if !pass && f.Status == "pass" {
					t.Fatal("failing development floor passed")
				}
			}
		})
	}
}

func TestReport_NoSealedEntry(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: 11, Day: 9}
	r, _, err := Run(context.Background(), &recordedEngine{fakeEngine: fakeEngine{date: date}}, developmentSuite(), Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "sealed") {
		t.Fatal("report contains removed cohort")
	}
}

type developmentResultEngine struct {
	recordedEngine
	miss bool
}

func (e *developmentResultEngine) Collect(ctx context.Context, p retrieval.Prepared, d retrieval.Depths) (retrieval.Arms, error) {
	development := p.Request.Query == "Câu hỏi phát triển"
	if development {
		p.Request.Query = "Câu hỏi tốt"
	}
	arms, err := e.recordedEngine.Collect(ctx, p, d)
	if development && e.miss {
		for _, candidates := range [][]retrieval.Candidate{arms.Primary.Dense, arms.Primary.Lexical, arms.Primary.Members, arms.Primary.RawANN} {
			for i := range candidates {
				candidates[i] = lossCandidate(2, "other", "one")
			}
		}
	}
	return arms, err
}

func TestDevelopment_NonRegressionIsIndependent(t *testing.T) {
	s := developmentSuite()
	s.Development[0].Question = "Câu hỏi phát triển"
	date := retrieval.Date{Year: 2026, Month: 11, Day: 9}
	baseline, _, err := Run(context.Background(), &developmentResultEngine{recordedEngine: recordedEngine{fakeEngine: fakeEngine{date: date}}}, s, Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err = baseline.WriteFiles(dir); err != nil {
		t.Fatal(err)
	}
	options := Options{EvaluationDate: date, PreviousReport: filepath.Join(dir, "report.json")}
	r, _, err := Run(context.Background(), &developmentResultEngine{recordedEngine: recordedEngine{fakeEngine: fakeEngine{date: date}}, miss: true}, s, options)
	if err != nil {
		t.Fatal(err)
	}
	if r.QualityPass {
		t.Fatal("development regression passed quality")
	}
	for _, comparison := range r.NonRegression {
		if comparison.Status != "pass" {
			t.Fatal("development changed old product comparison")
		}
	}
	summary := readDevelopmentSummary(t, r)
	for _, f := range summary.Floors {
		if f.Status != "pass" {
			t.Fatal("test must fail on regression, not floors")
		}
	}
	for _, comparison := range summary.NonRegression {
		if comparison.Name == "abstention_balanced_accuracy" {
			continue
		}
		if comparison.Status != "fail" || comparison.Value == nil || *comparison.Value != 0 || comparison.PreviousValue == nil || *comparison.PreviousValue != 1 {
			t.Fatalf("comparison: %+v", comparison)
		}
	}
	s.Hashes.Development = "changed"
	if _, _, err = Run(context.Background(), &recordedEngine{fakeEngine: fakeEngine{date: date}}, s, options); err == nil {
		t.Fatal("changed development fixture compared")
	}
}

func TestDevelopment_MappingUnavailableIsIndependent(t *testing.T) {
	s := developmentSuite()
	date := retrieval.Date{Year: 2026, Month: 11, Day: 9}
	o := Options{EvaluationDate: date, Mapping: ValidationReport{"development-positive": {{CitationIndex: 0, Kind: "mapping_incomplete", Field: "entry", Detail: "missing"}}}}
	r, _, err := Run(context.Background(), &recordedEngine{fakeEngine: fakeEngine{date: date}}, s, o)
	if err != nil {
		t.Fatal(err)
	}
	if r.QualityPass {
		t.Fatal("incomplete development mapping passed")
	}
	for _, target := range r.Targets {
		if target.Status != "met" {
			t.Fatal("development mapping blocked old product")
		}
	}
	for _, comparison := range readDevelopmentSummary(t, r).NonRegression {
		if comparison.Name != "abstention_balanced_accuracy" && comparison.Status != "unavailable" {
			t.Fatal("incomplete mapping has available comparison")
		}
	}
}

func TestSuite_RejectsDevelopmentRejectionID(t *testing.T) {
	s := developmentSuite()
	s.Rejections[0] = s.Development[0].ID
	if err := validateSuite(s); err == nil {
		t.Fatal("development case accepted as legacy rejection check")
	}
}
