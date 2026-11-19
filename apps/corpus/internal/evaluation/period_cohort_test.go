package evaluation

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

const periodQuestion = "Câu hỏi thời kỳ"

func periodSuite() Suite {
	s := developmentSuite()
	s.Development = append(s.Development,
		Case{ID: "period-positive", Question: periodQuestion, ExpectedCitations: []Expectation{{DocNumber: "doc"}}, Tags: []string{"historical", "multi_document"}},
		Case{ID: "transition-positive", Question: "Câu hỏi tốt", ExpectedCitations: []Expectation{{DocNumber: "doc"}}, Tags: []string{"identifier", "transition"}},
	)
	for _, id := range []string{"period-positive", "transition-positive"} {
		entry := s.Map.Entries[0]
		entry.CaseID = id
		s.Map.Entries = append(s.Map.Entries, entry)
	}
	return s
}

type periodEngine struct {
	recordedEngine
	mu       sync.Mutex
	miss     bool
	requests map[string][]bool
}

func (e *periodEngine) Prepare(ctx context.Context, r retrieval.Request) (retrieval.Prepared, error) {
	e.mu.Lock()
	if e.requests == nil {
		e.requests = map[string][]bool{}
	}
	e.requests[r.Query] = append(e.requests[r.Query], r.InForceOnly != nil && *r.InForceOnly && r.AsOf == nil)
	e.mu.Unlock()
	return e.recordedEngine.Prepare(ctx, r)
}

func (e *periodEngine) Collect(ctx context.Context, p retrieval.Prepared, d retrieval.Depths) (retrieval.Arms, error) {
	period := p.Request.Query == periodQuestion
	if period {
		p.Request.Query = "Câu hỏi tốt"
	}
	arms, err := e.recordedEngine.Collect(ctx, p, d)
	if period && e.miss {
		arms.Primary = retrieval.ArmGroup{}
	}
	return arms, err
}

func caseIDs(r Report, cohort string) []string {
	seen := map[string]bool{}
	for _, c := range r.Cases {
		if c.Cohort == cohort {
			seen[c.CaseID] = true
		}
	}
	var out []string
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func TestPeriodCohort_PartitionsDevelopmentCases(t *testing.T) {
	date := retrieval.Date{Year: 2099, Month: 1, Day: 2}
	s := periodSuite()
	r, cache, err := Run(context.Background(), &periodEngine{recordedEngine: recordedEngine{fakeEngine: fakeEngine{date: date}}}, s, Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	development, period := caseIDs(r, "development"), caseIDs(r, "development_period")
	if want := []string{"development-negative", "development-positive", "transition-positive"}; !equalStrings(development, want) {
		t.Fatalf("development = %v, want %v", development, want)
	}
	if want := []string{"period-positive"}; !equalStrings(period, want) {
		t.Fatalf("development_period = %v, want %v", period, want)
	}
	if r.Cohorts["development"].CaseCount != 3 || r.Cohorts["development_period"].CaseCount != 1 || r.Cohorts["development_period"].PositiveCount != 1 {
		t.Fatalf("cohorts = %+v", r.Cohorts)
	}
	if _, ok := r.NotRun["development_period"]; ok {
		t.Fatalf("not_run = %v", r.NotRun)
	}
	if _, ok := r.NotRun["development"]; ok {
		t.Fatalf("not_run = %v", r.NotRun)
	}
	indexed := 0
	for _, entry := range cache.Index {
		if entry.Cohort == "development_period" && entry.CaseID == "period-positive" {
			indexed++
		}
	}
	if indexed != 1 {
		t.Fatalf("period cache entries = %d", indexed)
	}
	if len(r.Cohorts["development_period"].Floors) != 0 || len(r.Cohorts["development_period"].Targets) != 0 {
		t.Fatal("period cohort has floors or targets")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestPeriodCohort_EmptyIsNotRun(t *testing.T) {
	date := retrieval.Date{Year: 2099, Month: 1, Day: 2}
	r, _, err := Run(context.Background(), &recordedEngine{fakeEngine: fakeEngine{date: date}}, developmentSuite(), Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	period, ok := r.Cohorts["development_period"]
	if !ok || period.CaseCount != 0 || len(period.NonRegression) != 0 || r.NotRun["development_period"] != "not_run" {
		t.Fatalf("period = %+v, not_run = %v", period, r.NotRun)
	}
	if !r.QualityPass {
		t.Fatal("empty period cohort changed quality")
	}
}

func TestPeriodCohort_PeriodRequest(t *testing.T) {
	date := retrieval.Date{Year: 2099, Month: 1, Day: 2}
	engine := &periodEngine{recordedEngine: recordedEngine{fakeEngine: fakeEngine{date: date}}}
	s := periodSuite()
	s.Development[0].Question = "Câu hỏi phát triển"
	if _, _, err := Run(context.Background(), engine, s, Options{EvaluationDate: date}); err != nil {
		t.Fatal(err)
	}
	for question, want := range map[string]bool{periodQuestion: false, "Câu hỏi phát triển": true, "Câu hỏi không": true, "Câu hỏi tốt": true} {
		requests := engine.requests[question]
		if len(requests) == 0 {
			t.Fatalf("%s: no request", question)
		}
		for _, inForce := range requests {
			if inForce != want {
				t.Errorf("%s: default request = %v, want %v", question, inForce, want)
			}
		}
	}
}

func TestPeriodCohort_ReportedNotGated(t *testing.T) {
	date := retrieval.Date{Year: 2099, Month: 1, Day: 2}
	s := periodSuite()
	baseline, _, err := Run(context.Background(), &periodEngine{recordedEngine: recordedEngine{fakeEngine: fakeEngine{date: date}}}, s, Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err = baseline.WriteFiles(dir); err != nil {
		t.Fatal(err)
	}
	r, _, err := Run(context.Background(), &periodEngine{recordedEngine: recordedEngine{fakeEngine: fakeEngine{date: date}}, miss: true}, s, Options{EvaluationDate: date, PreviousReport: filepath.Join(dir, "report.json")})
	if err != nil {
		t.Fatal(err)
	}
	if !r.QualityPass {
		t.Fatal("period cohort fall failed quality")
	}
	records := r.Cohorts["development_period"].NonRegression
	if len(records) != 6 {
		t.Fatalf("period records = %+v", records)
	}
	seen := map[string]bool{}
	for _, n := range records {
		seen[fmt.Sprintf("%s:%s:%d", n.Mode, n.Name, n.Limit)] = true
		if n.Gating || n.Status != "reported" || n.Arms != "hybrid" || n.MRRComparison != nil || n.Revision != nil {
			t.Fatalf("record = %+v", n)
		}
		if n.Value == nil || *n.Value != 0 || n.PreviousValue == nil || *n.PreviousValue != 1 {
			t.Fatalf("values = %+v", n)
		}
	}
	for _, key := range []string{"exact:recall:20", "exact:recall:8", "exact:mrr:8", "ann:recall:20", "ann:recall:8", "ann:mrr:8"} {
		if !seen[key] {
			t.Fatalf("missing %s in %v", key, seen)
		}
	}
	for _, n := range baseline.Cohorts["development_period"].NonRegression {
		if n.Status != "reported" || n.PreviousValue != nil {
			t.Fatalf("baseline record = %+v", n)
		}
	}
}

func TestCalibrate_FitsPeriodCasesUnderDefaultRequest(t *testing.T) {
	date := retrieval.Date{Year: 2099, Month: 1, Day: 2}
	engine := &periodEngine{recordedEngine: recordedEngine{fakeEngine: fakeEngine{date: date}}}
	calibration, err := Calibrate(context.Background(), engine, periodSuite())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, score := range calibration.Scores {
		found = found || score.CaseID == "period-positive"
	}
	if !found {
		t.Fatal("calibration left out the period case")
	}
	for _, inForce := range engine.requests[periodQuestion] {
		if !inForce {
			t.Fatal("calibration sent the period request")
		}
	}
}

func TestPeriodCohort_PreviousValueFromPeriodCohort(t *testing.T) {
	date := retrieval.Date{Year: 2099, Month: 1, Day: 2}
	s := periodSuite()
	baseline, _, err := Run(context.Background(), &periodEngine{recordedEngine: recordedEngine{fakeEngine: fakeEngine{date: date}}, miss: true}, s, Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	r, _, err := Run(context.Background(), &periodEngine{recordedEngine: recordedEngine{fakeEngine: fakeEngine{date: date}}}, s, Options{EvaluationDate: date, PreviousReport: previousReportPath(t, baseline)})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range readDevelopmentSummary(t, baseline).NonRegression {
		if n.Value == nil || *n.Value != 1 {
			t.Fatalf("baseline development record = %+v", n)
		}
	}
	records := r.Cohorts["development_period"].NonRegression
	if len(records) != 6 {
		t.Fatalf("period records = %+v", records)
	}
	for _, n := range records {
		if n.Value == nil || *n.Value != 1 || n.PreviousValue == nil || *n.PreviousValue != 0 {
			t.Fatalf("%s %s@%d: value %v previous %v", n.Mode, n.Name, n.Limit, n.Value, n.PreviousValue)
		}
	}
}

func TestPeriodCohort_ArmRowsUsePeriodEligibility(t *testing.T) {
	date := retrieval.Date{Year: 2099, Month: 1, Day: 2}
	_, cache, err := Run(context.Background(), &periodEngine{recordedEngine: recordedEngine{fakeEngine: fakeEngine{date: date}}}, periodSuite(), Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	current, period := 0, 0
	for _, row := range cache.Arms {
		if row.Cohort == "development_period" {
			period++
			if row.EligibilityHash == eligibilityHash(date) {
				t.Fatalf("period row %s %s carries the in-force eligibility hash", row.CaseID, row.Mode)
			}
			continue
		}
		current++
		if row.EligibilityHash != eligibilityHash(date) {
			t.Fatalf("%s row %s %s eligibility hash differs", row.Cohort, row.CaseID, row.Mode)
		}
	}
	if period != 2 || current == 0 {
		t.Fatalf("arm rows: %d period, %d other", period, current)
	}
}
