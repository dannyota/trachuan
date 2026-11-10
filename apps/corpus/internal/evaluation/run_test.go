package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
)

type fakeEngine struct {
	prepares, legacy, collects int
	fail                       bool
	date                       retrieval.Date
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
	return Suite{Legacy: cases, Map: m, Rejections: product.Rejections, Hashes: FixtureHashes{Legacy: m.LegacySHA256, Inventory: m.IdentityInventoryHash}}
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
	if len(report.Floors) != 8 {
		t.Fatalf("floors = %d, want 8", len(report.Floors))
	}
	count := 0
	for _, f := range report.Floors {
		if f.Mode == "ann" && f.Status == "unavailable" {
			count++
		}
	}
	if count != 4 {
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
	for _, floor := range report.Floors {
		if floor.Name == "recall" && floor.Status != "unavailable" {
			t.Fatalf("recall status = %s", floor.Status)
		}
	}
}
