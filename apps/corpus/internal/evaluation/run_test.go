package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
)

type fakeEngine struct {
	prepares, legacy, collects int
	fail                       bool
	date                       retrieval.Date
	compactText, fullText      string
}

func (f *fakeEngine) Present(_ context.Context, _ retrieval.Response, d retrieval.Detail) (retrieval.SearchResult, error) {
	value := f.compactText
	if d == retrieval.DetailFull {
		value = f.fullText
	}
	return retrieval.SearchResult{Release: string(d) + " Việt" + value}, nil
}

type parentHitEngine struct{ fakeEngine }

func (f *parentHitEngine) Collect(_ context.Context, p retrieval.Prepared, d retrieval.Depths) (retrieval.Arms, error) {
	arms := retrieval.Arms{Validation: p.Validation, Depths: d, Date: f.date}
	if p.Request.Query != "Câu hỏi tốt" {
		return arms, nil
	}
	hit := lossCandidate(1, "doc", "parent")
	parentID := hit.GroupID
	hit.Descendants = append(hit.Descendants, retrieval.Provision{ID: 2, ParentID: &parentID, NodeID: "child", Kind: "diem", Text: "child text", Identity: retrieval.Identity{Country: "vn", DocumentKey: "doc", Anchor: "child"}})
	arms.Primary.Dense = []retrieval.Candidate{hit}
	arms.Primary.Lexical = []retrieval.Candidate{hit}
	arms.Primary.Members = []retrieval.Candidate{hit}
	return arms, nil
}

func TestRun_CompactDiagnosticOwnIdentityOnly(t *testing.T) {
	child, parent := "child", "parent"
	suite := recordedSuite()
	suite.Legacy[0].ExpectedCitations = append(suite.Legacy[0].ExpectedCitations, Expectation{DocNumber: "doc"})
	suite.Map.Entries[0].Alternatives = []Alternative{{Country: "vn", DocumentKey: "doc", Anchor: &child, Kind: "provision"}}
	own := suite.Map.Entries[0]
	own.CitationIndex = 1
	own.Alternatives = []Alternative{{Country: "vn", DocumentKey: "doc", Anchor: &parent, Kind: "provision"}}
	suite.Map.Entries = append(suite.Map.Entries, own)
	date := retrieval.Date{Year: 2026, Month: time.November, Day: 9}
	r, _, err := Run(context.Background(), &parentHitEngine{fakeEngine{date: date}}, suite, Options{EvaluationDate: date, Modes: []retrieval.Mode{"exact"}})
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, s := range r.Presented {
		if s.Cohort != "old_product" || s.Mode != "exact" {
			continue
		}
		want := 1
		if s.Detail == retrieval.DetailFull {
			want = 2
		}
		if s.Found != want || s.Expected != 2 {
			t.Fatalf("summary: %+v", s)
		}
		checked++
	}
	if checked != 4 {
		t.Fatalf("checked %d summaries, want 4", checked)
	}
}

func TestRun_PresentedSizeIsSerializedResult(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: time.November, Day: 9}
	r, _, err := Run(context.Background(), &fakeEngine{date: date}, recordedSuite(), Options{EvaluationDate: date, Modes: []retrieval.Mode{"exact"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Presented) != 8 {
		t.Fatalf("summaries: %d", len(r.Presented))
	}
	for _, s := range r.Presented {
		b, err := json.Marshal(retrieval.SearchResult{Release: string(s.Detail) + " Việt"})
		if err != nil {
			t.Fatal(err)
		}
		want := float64(utf8.RuneCount(b))
		if s.SizeP50 == nil || s.SizeP95 == nil || *s.SizeP50 != want || *s.SizeP95 != want {
			t.Fatalf("summary: %+v want %v", s, want)
		}
	}
}

func TestRun_CompactSizeGate(t *testing.T) {
	for _, tc := range []struct {
		full, compact float64
		pass          bool
	}{{10, 10, true}, {10, 9, true}, {10, 11, false}} {
		r := Report{Presented: []PresentedSummary{{Cohort: "old_product", Mode: "exact", Detail: retrieval.DetailFull, Limit: 8, SizeP95: &tc.full}, {Cohort: "old_product", Mode: "exact", Detail: retrieval.DetailCompact, Limit: 20, SizeP95: &tc.compact}}, QualityPass: true, ReleaseEligible: false, Floors: []Floor{{Status: "pass"}}}
		setCompactSizeGate(&r)
		if r.CompactSizeGate.Pass != tc.pass || !r.QualityPass || r.ReleaseEligible || r.Floors[0].Status != "pass" {
			t.Fatalf("gate: %+v", r.CompactSizeGate)
		}
	}
	date := retrieval.Date{Year: 2026, Month: time.November, Day: 9}
	var baseline Report
	for i, extra := range []string{"", strings.Repeat("x", 100)} {
		engine := &recordedEngine{fakeEngine: fakeEngine{date: date, compactText: extra, fullText: strings.Repeat("x", 50)}}
		r, _, err := Run(context.Background(), engine, recordedSuite(), Options{EvaluationDate: date})
		if err != nil {
			t.Fatal(err)
		}
		if r.CompactSizeGate.Pass != (i == 0) || r.CompactSizeGate.FullP95At8 == nil || r.CompactSizeGate.CompactP95At20 == nil {
			t.Fatalf("gate: %+v", r.CompactSizeGate)
		}
		if i == 0 {
			baseline = r
		} else if !reflect.DeepEqual(baseline.Floors, r.Floors) || baseline.QualityPass != r.QualityPass || baseline.ReleaseEligible != r.ReleaseEligible || !reflect.DeepEqual(baseline.Cases, r.Cases) {
			t.Fatal("size gate changed scoring or promotion")
		}
	}
}

func (f *fakeEngine) Release() retrieval.Release {
	threshold := 0.0
	recipe := retrieval.Recipe1()
	recipe.Abstention.Threshold = &threshold
	return retrieval.Release{Country: "vn", Rel: "test", RetrievalRecipe: recipe}
}
func (f *fakeEngine) Prepare(_ context.Context, r retrieval.Request) (retrieval.Prepared, error) {
	f.prepares++
	v, err := retrieval.ValidateQuery(r.Query)
	if err != nil {
		return retrieval.Prepared{}, err
	}
	p := retrieval.Prepared{Request: r, Validation: v}
	if v.Accepted {
		p.Vector = &embedding.Vector{Values: []float32{1}, TokenCount: 1, RecipeHash: "query"}
	}
	return p, nil
}
func (f *fakeEngine) PrepareLegacy(ctx context.Context, r retrieval.Request) (retrieval.Prepared, error) {
	f.legacy++
	p, err := f.Prepare(ctx, r)
	if err == nil && !p.Validation.Accepted {
		p.Validation.Accepted = true
		p.Validation.Reason = ""
		p.Validation.Message = ""
		p.Vector = &embedding.Vector{Values: []float32{1}, TokenCount: 1, RecipeHash: "query"}
	}
	return p, err
}
func (f *fakeEngine) Collect(_ context.Context, p retrieval.Prepared, d retrieval.Depths) (retrieval.Arms, error) {
	f.collects++
	if f.fail {
		return retrieval.Arms{}, errors.New("offline failure")
	}
	return retrieval.Arms{Validation: p.Validation, Depths: d, Date: f.date}, nil
}
func testSuite(t *testing.T) Suite {
	t.Helper()
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join("testdata/vn", name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	cases, err := LoadLegacy(strings.NewReader(string(read("legacy.json"))))
	if err != nil {
		t.Fatal(err)
	}
	m, err := LoadIdentityMap(strings.NewReader(string(read("identity-map.json"))))
	if err != nil {
		t.Fatal(err)
	}
	var product struct {
		Rejections []string `json:"rejection_ids"`
	}
	if err = json.Unmarshal(read("old-product.json"), &product); err != nil {
		t.Fatal(err)
	}
	return Suite{Legacy: cases, Map: m, Rejections: product.Rejections, Hashes: FixtureHashes{Legacy: m.LegacySHA256}}
}
func TestRun_RejectionsVectorsAndFiles(t *testing.T) {
	suite := testSuite(t)
	date := retrieval.Date{Year: 2026, Month: time.November, Day: 9}
	engine := &fakeEngine{date: date}
	report, cache, err := Run(context.Background(), engine, suite, Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	if engine.legacy != 3 || engine.prepares != 95 {
		t.Fatalf("prepares %d legacy %d", engine.prepares, engine.legacy)
	}
	if len(cache.Vectors) != len(suite.Legacy) {
		t.Fatalf("vectors %d", len(cache.Vectors))
	}
	if report.ReleaseEligible || report.QualityPass || len(report.Rejections) != 3 {
		t.Fatalf("unexpected gates: %+v", report.Rejections)
	}
	if report.Cohorts["old_product"].RejectionCount != 3 || report.Cohorts["old_product"].PositiveCount != 84 {
		t.Fatal("incorrect cohort counts")
	}
	dir := t.TempDir()
	if err = report.WriteFiles(dir); err != nil {
		t.Fatal(err)
	}
	if err = cache.WriteFiles(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"report.json", "cases.jsonl", "arms.jsonl", "queries.f32", "query-index.json"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range suite.Legacy {
			if strings.Contains(string(b), c.Question) {
				t.Fatalf("question in %s", name)
			}
		}
	}
}
func TestRun_InfrastructureFailureIsIncomplete(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: time.November, Day: 9}
	_, _, err := Run(context.Background(), &fakeEngine{date: date, fail: true}, testSuite(t), Options{EvaluationDate: date})
	if err == nil {
		t.Fatal("expected collect error")
	}
}

func TestANNOverlap_UsesRecipeDepth(t *testing.T) {
	exact := make([]retrieval.Candidate, 200)
	for i := range exact {
		exact[i].ChunkID = int64(i + 1)
	}
	raw := []retrieval.Candidate{{ChunkID: 100}, {ChunkID: 1}}
	got := annMeasure("id", "legacy", exact, raw, raw, false, 50)
	if got.ExactCount != 50 || got.RawOverlap != 1 {
		t.Fatalf("unexpected overlap: %+v", got)
	}
}

func TestPreviousDiff_ReportsCaseGainsAndLosses(t *testing.T) {
	previous := []CaseResult{{CaseID: "a", Cohort: "legacy", Mode: "exact", Arms: "hybrid", Limit: 8, Ranks: []int{0, 1}}}
	current := []CaseResult{{CaseID: "a", Cohort: "legacy", Mode: "exact", Arms: "hybrid", Limit: 8, Ranks: []int{1, 0}}}
	changes := compareCases(previous, current)
	if len(changes) != 1 || len(changes[0].Gained) != 1 || len(changes[0].Lost) != 1 {
		t.Fatalf("changes: %+v", changes)
	}
}

func TestANNLoss_ReportsModeDifferences(t *testing.T) {
	r := Report{Cohorts: map[string]Cohort{"old_product": {Metrics: []Metric{{Mode: "exact", Arms: "hybrid", Limit: 8, Name: "recall", Value: floatPtr(.8)}, {Mode: "ann", Arms: "hybrid", Limit: 8, Name: "recall", Value: floatPtr(.7)}}}}, ANN: []ANNMeasure{{ExactCount: 2, RawOverlap: 1, EffectiveOverlap: 2, Fallback: true}}}
	loss := summarizeANN(r, "old_product")
	if loss.RawRecall == nil || *loss.RawRecall != .5 || loss.Fallbacks != 1 || loss.Recall8PP == nil || *loss.Recall8PP < 9.99 {
		t.Fatalf("loss: %+v", loss)
	}
}
func floatPtr(v float64) *float64 { return &v }

func TestRun_MissingModeFloorsUnavailable(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: time.November, Day: 9}
	report, _, err := Run(context.Background(), &fakeEngine{date: date}, testSuite(t), Options{EvaluationDate: date, Modes: []retrieval.Mode{"exact"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Floors) != 4 {
		t.Fatalf("floors = %d, want 4", len(report.Floors))
	}
	count := 0
	for _, f := range report.Floors {
		if f.Mode == "ann" && f.Status == "unavailable" {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("unavailable ANN floors = %d", count)
	}
}

func TestReport_CacheBudgetIncludesExistingVectors(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "queries.f32"))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(cacheLimit); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if err = (Report{}).WriteFiles(dir); err == nil {
		t.Fatal("expected budget failure")
	}
}

func TestLegacyDirectMetrics_UseDirectRanks(t *testing.T) {
	r := Report{Cohorts: map[string]Cohort{"legacy": {}}, Cases: []CaseResult{{CaseID: "a", Cohort: "legacy", Mode: "exact", Arms: "hybrid", Limit: 8, Expected: 2, Ranks: []int{1, 2}, DirectRanks: []int{1, 0}}}}
	addLegacyDirectMetrics(&r)
	for _, m := range r.Cohorts["legacy"].Metrics {
		if m.Name == "direct_recall" && m.Value != nil && *m.Value == .5 {
			return
		}
	}
	t.Fatal("missing direct recall")
}

func TestRun_TimingSummariesPresent(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: time.November, Day: 9}
	report, _, err := Run(context.Background(), &fakeEngine{date: date}, testSuite(t), Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.TimingsMS) == 0 {
		t.Fatal("missing timing summaries")
	}
}

func TestCache_KeyChangesWithVectorAndInputs(t *testing.T) {
	a := Cache{Vectors: [][]float32{{1}}}
	b := Cache{Vectors: [][]float32{{.5}}}
	x := cacheKey(a, Options{DumpSHA256: "dump"}, FixtureHashes{})
	if x == cacheKey(b, Options{DumpSHA256: "dump"}, FixtureHashes{}) || x == cacheKey(a, Options{DumpSHA256: "other"}, FixtureHashes{}) {
		t.Fatal("cache key did not change")
	}
}

func TestRun_RecordsReleaseRecipeHashes(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: time.November, Day: 9}
	engine := &fakeEngine{date: date}
	r, _, err := Run(context.Background(), engine, testSuite(t), Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	if r.Recipes["retrieval"] != engine.Release().RetrievalRecipe.Hash() {
		t.Fatal("missing retrieval recipe hash")
	}
}

func TestCaseResult_RecordsMappedIdentities(t *testing.T) {
	entry := MapEntry{Alternatives: []Alternative{{Country: "vn", DocumentKey: "doc", Kind: "document"}}}
	got := caseMappings(map[int]MapEntry{0: entry}, 1)
	if len(got) != 1 || len(got[0]) != 1 || got[0][0].DocumentKey != "doc" {
		t.Fatal("missing mapped identity")
	}
}

func TestRun_CompletedWithLossDiagnostics(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: time.November, Day: 9}
	r, _, err := Run(context.Background(), &fakeEngine{date: date}, testSuite(t), Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "complete" {
		t.Fatal("expected a complete report")
	}
}

func TestRun_MappingIncompleteScoresOtherCases(t *testing.T) {
	suite := testSuite(t)
	date := retrieval.Date{Year: 2026, Month: time.November, Day: 9}
	id := suite.Legacy[0].ID
	issues := ValidationReport{id: {{CitationIndex: 0, Kind: "mapping_incomplete", Field: "alternatives[0].document_number", Detail: "missing"}}}
	report, _, err := Run(context.Background(), &fakeEngine{date: date}, suite, Options{EvaluationDate: date, Mapping: issues})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, result := range report.Cases {
		if result.CaseID == id && result.Cohort == "old_product" {
			found = len(result.MappingIssues) > 0
		}
	}
	if !found {
		t.Fatal("missing per-case issue")
	}
	if report.QualityPass {
		t.Fatal("incomplete mapping passed quality")
	}
	for _, target := range report.Targets {
		if target.Status != "unavailable" {
			t.Fatalf("target: %+v", target)
		}
	}
	for _, regression := range report.NonRegression {
		if regression.Status != "unavailable" {
			t.Fatalf("regression: %+v", regression)
		}
	}
}

func TestRun_ReportsTargetsAndNonRegression(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: time.November, Day: 9}
	r, _, err := Run(context.Background(), &recordedEngine{fakeEngine: fakeEngine{date: date}}, recordedSuite(), Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Targets []struct {
			Status string `json:"status"`
		} `json:"targets"`
		NonRegression []struct {
			Status string `json:"status"`
		} `json:"non_regression"`
	}
	if err := json.Unmarshal(b, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Targets) != 6 || len(result.NonRegression) != 4 {
		t.Fatalf("targets %d non-regression %d", len(result.Targets), len(result.NonRegression))
	}
	for _, target := range result.Targets {
		if target.Status != "met" {
			t.Fatalf("target status %s", target.Status)
		}
	}
	for _, regression := range result.NonRegression {
		if regression.Status != "no_previous" {
			t.Fatalf("regression status %s", regression.Status)
		}
	}
}

func TestQuality_TargetsGatesAndRegression(t *testing.T) {
	metric := func(mode retrieval.Mode, name string, limit int, value *float64) Metric {
		return Metric{Mode: mode, Arms: "hybrid", Name: name, Limit: limit, Value: value}
	}
	makeReport := func(recall, mrr, precision, abstention *float64) Report {
		r := Report{Cohorts: map[string]Cohort{}, Rejections: []Rejection{{Passed: true}, {Passed: true}, {Passed: true}}}
		var metrics []Metric
		for _, mode := range []retrieval.Mode{"exact", "ann"} {
			metrics = append(metrics, metric(mode, "recall", 20, recall), metric(mode, "recall", 8, recall), metric(mode, "mrr", 8, mrr), metric(mode, "current_law_precision", 8, precision), metric(mode, "abstention", 8, abstention))
		}
		r.Cohorts["old_product"] = Cohort{Metrics: metrics}
		return r
	}
	for _, tc := range []struct {
		name               string
		value, prior       *float64
		target, regression string
		quality            bool
	}{
		{"below target", floatPtr(.5), nil, "below", "no_previous", true},
		{"met equal", floatPtr(.9), floatPtr(.9), "met", "pass", true},
		{"regression", floatPtr(.8), floatPtr(.9), "below", "fail", false},
		{"missing current", nil, floatPtr(.9), "unavailable", "unavailable", false},
		{"missing previous metric", floatPtr(.9), nil, "met", "no_previous", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := makeReport(tc.value, floatPtr(.7), floatPtr(1), floatPtr(1))
			prev := makeReport(tc.prior, floatPtr(.7), floatPtr(1), floatPtr(1))
			finishQuality(&r, &prev)
			if r.Targets[0].Status != tc.target || r.NonRegression[0].Status != tc.regression || r.QualityPass != tc.quality {
				t.Fatalf("target %s regression %+v quality %v", r.Targets[0].Status, r.NonRegression[0], r.QualityPass)
			}
			if !reflect.DeepEqual(r.NonRegression[0].Value, tc.value) || !reflect.DeepEqual(r.NonRegression[0].PreviousValue, tc.prior) {
				t.Fatalf("comparison values %+v", r.NonRegression[0])
			}
		})
	}
	for _, tc := range []struct {
		name    string
		value   *float64
		status  string
		quality bool
	}{{"pass", floatPtr(.99), "pass", true}, {"fail", floatPtr(.98), "fail", false}, {"unavailable", nil, "unavailable", false}} {
		t.Run(tc.name, func(t *testing.T) {
			r := makeReport(floatPtr(1), floatPtr(1), tc.value, floatPtr(1))
			finishQuality(&r, nil)
			if r.Floors[0].Status != tc.status || r.QualityPass != tc.quality {
				t.Fatalf("gate %+v quality %v", r.Floors[0], r.QualityPass)
			}
		})
	}
	r := makeReport(floatPtr(1), floatPtr(1), floatPtr(1), floatPtr(1))
	markMappingUnavailable(&r)
	finishQuality(&r, nil)
	if r.QualityPass || r.Targets[0].Status != "unavailable" || r.NonRegression[0].Status != "unavailable" {
		t.Fatalf("mapping: %+v", r)
	}
}

func TestRun_PreviousReportAffectsQuality(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: time.November, Day: 9}
	opts := Options{EvaluationDate: date}
	engine := &recordedEngine{fakeEngine: fakeEngine{date: date}}
	prior, _, err := Run(context.Background(), engine, recordedSuite(), opts)
	if err != nil {
		t.Fatal(err)
	}
	for i := range prior.Cohorts["old_product"].Metrics {
		m := &prior.Cohorts["old_product"].Metrics[i]
		if m.Name == "recall" && m.Limit == 20 && m.Arms == "hybrid" {
			m.Value = floatPtr(1.1)
		}
	}
	b, err := json.Marshal(prior)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "previous.json")
	if err = os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	opts.PreviousReport = path
	r, _, err := Run(context.Background(), engine, recordedSuite(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if r.QualityPass || r.NonRegression[0].Status != "fail" || *r.NonRegression[0].PreviousValue != 1.1 {
		t.Fatalf("regression %+v quality %v", r.NonRegression, r.QualityPass)
	}
}
