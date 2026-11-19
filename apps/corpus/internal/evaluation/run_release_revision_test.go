package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
	"danny.vn/trachuan/internal/retrieval/lexical"
)

type releaseEngine struct {
	periodEngine
	rel   string
	ranks map[string]int
}

var releaseChunkRecipe = json.RawMessage(`{"version":1}`)

func releaseEmbedding() embedding.Recipe {
	return embedding.Recipe{Model: "synthetic", Revision: "test", Dimensions: 1, Pooling: "mean", Normalization: "l2", Input: "text", Runtime: "test", Backend: "test", Precision: "fp32", TokenCap: 8, Language: "vi"}
}

func (e *releaseEngine) Release() retrieval.Release {
	r := e.periodEngine.Release()
	r.Rel = e.rel
	r.ChunkRecipe = releaseChunkRecipe
	r.LexicalRecipe = lexical.Recipe1()
	recipe := releaseEmbedding()
	r.DenseRecipe = retrieval.DenseRecipe{Version: 1, Model: recipe.Model, Dimensions: recipe.Dimensions, Document: recipe, DocumentHash: recipe.Hash(), Query: recipe, QueryHash: recipe.Hash()}
	return r
}

func (e *releaseEngine) Collect(ctx context.Context, p retrieval.Prepared, d retrieval.Depths) (retrieval.Arms, error) {
	rank := e.ranks[p.Request.Query]
	if rank == 0 {
		return e.periodEngine.Collect(ctx, p, d)
	}
	p.Request.Query = "Câu hỏi tốt"
	arms, err := e.periodEngine.Collect(ctx, p, d)
	for _, list := range []*[]retrieval.Candidate{&arms.Primary.Dense, &arms.Primary.Lexical, &arms.Primary.Members, &arms.Primary.RawANN} {
		if len(*list) == 0 {
			continue
		}
		var ranked []retrieval.Candidate
		for i := 1; i <= 10; i++ {
			if i == rank {
				ranked = append(ranked, *list...)
			}
			ranked = append(ranked, lossCandidate(int64(100+i), fmt.Sprintf("filler-%d", i), "one"))
		}
		*list = ranked
	}
	return arms, err
}

func releaseRow(t *testing.T, rel retrieval.Release) map[string]any {
	t.Helper()
	dense, err := rel.DenseRecipe.Query.CompactJSON()
	if err != nil {
		t.Fatal(err)
	}
	lexicalJSON, err := rel.LexicalRecipe.JSON()
	if err != nil {
		t.Fatal(err)
	}
	retrievalJSON, err := rel.RetrievalRecipe.JSON()
	if err != nil {
		t.Fatal(err)
	}
	return map[string]any{
		"dense_recipe":   map[string]any{"version": 1, "model": rel.DenseRecipe.Model, "dimensions": rel.DenseRecipe.Dimensions, "query": json.RawMessage(dense), "document": json.RawMessage(dense), "query_hash": rel.DenseRecipe.QueryHash, "document_hash": rel.DenseRecipe.DocumentHash},
		"chunk_recipe":   rel.ChunkRecipe,
		"lexical_recipe": json.RawMessage(lexicalJSON), "retrieval_recipe": json.RawMessage(retrievalJSON),
		"recipe_hash_inputs": map[string]string{"query": string(dense), "document": string(dense), "chunk": string(rel.ChunkRecipe), "lexical": string(lexicalJSON), "retrieval": string(retrievalJSON)},
	}
}

func releaseSuite(t *testing.T, cases []Case) (Suite, []byte) {
	t.Helper()
	s := recordedSuite()
	s.Hashes.OldProduct = "old-product-fixture"
	s.Development = cases
	for _, c := range cases {
		if len(c.ExpectedCitations) > 0 {
			entry := s.Map.Entries[0]
			entry.CaseID = c.ID
			s.Map.Entries = append(s.Map.Entries, entry)
		}
	}
	data, err := json.Marshal(cases)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	s.Hashes.Development = hex.EncodeToString(sum[:])
	return s, data
}

func writeReleaseFile(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func versionTwoPrevious(t *testing.T, path string, r Report) {
	t.Helper()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err = json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	object["version"], object["scorer"] = 2, "agent-eval/5"
	delete(object["cohorts"].(map[string]any), "development_period")
	delete(object["not_run"].(map[string]any), "development_period")
	writeReleaseFile(t, path, object)
}

func releaseFixtures(t *testing.T) (Suite, []byte, Suite, []byte) {
	t.Helper()
	doc := []Expectation{{DocNumber: "doc"}}
	base := []Case{
		{ID: "development-positive", Question: "Câu hỏi tốt", ExpectedCitations: doc, Tags: []string{"identifier"}},
		{ID: "development-negative", Question: "Câu hỏi không", ExpectAbstain: true, Tags: []string{"negative"}},
		{ID: "moving", Question: "Câu hỏi tốt", ExpectedCitations: doc, Tags: []string{"multi_document"}},
		{ID: "falling", Question: "Câu hỏi bảy", ExpectedCitations: doc, Tags: []string{"clause"}},
		{ID: "steady", Question: "Câu hỏi tốt", ExpectedCitations: doc, Tags: []string{"abbreviation"}},
	}
	revised := append([]Case(nil), base...)
	revised[0].Tags = []string{"identifier", "transition"}
	revised[2].Tags = []string{"historical", "multi_document"}
	revised = append(revised, Case{ID: "added", Question: "Câu hỏi tốt", ExpectedCitations: doc, Tags: []string{"identifier"}})
	previousSuite, previousFixture := releaseSuite(t, base)
	currentSuite, currentFixture := releaseSuite(t, revised)
	currentSuite.PreviousDevelopment = map[string][]byte{previousSuite.Hashes.Development: previousFixture}
	return previousSuite, previousFixture, currentSuite, currentFixture
}

func exactMRR(t *testing.T, r Report) NonRegression {
	t.Helper()
	return developmentRecord(t, r, "exact", "mrr", 8)
}

func TestReleaseValidator_AcceptsRevisedVersionThreeReports(t *testing.T) {
	ctx := context.Background()
	date := retrieval.Date{Year: 2099, Month: 1, Day: 2}
	engine := func(rel string, falling int) *releaseEngine {
		return &releaseEngine{periodEngine: periodEngine{recordedEngine: recordedEngine{fakeEngine: fakeEngine{date: date}}}, rel: rel, ranks: map[string]int{"Câu hỏi bảy": falling}}
	}
	previousSuite, previousFixture, currentSuite, currentFixture := releaseFixtures(t)
	dir := t.TempDir()
	base, _, err := Run(ctx, engine("20990101_1", 7), previousSuite, Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	versionTwoPrevious(t, filepath.Join(dir, "r0", "report.json"), base)
	reports := map[string]Report{}
	for _, run := range []struct {
		name, rel, previous string
		suite               Suite
	}{
		{"r1", "20990102_1", "r0", previousSuite},
		{"r2", "20990103_1", "r1", currentSuite},
		{"r3", "20990103_2", "r1", previousSuite},
		{"r4", "20990104_1", "r2", currentSuite},
	} {
		r, _, runErr := Run(ctx, engine(run.rel, 8), run.suite, Options{EvaluationDate: date, PreviousReport: filepath.Join(dir, run.previous, "report.json")})
		if runErr != nil {
			t.Fatal(runErr)
		}
		if r.Version != 3 || !r.QualityPass {
			t.Fatalf("%s: version %d quality %v", run.name, r.Version, r.QualityPass)
		}
		if err = r.WriteFiles(filepath.Join(dir, run.name)); err != nil {
			t.Fatal(err)
		}
		reports[run.name] = r
	}
	r1, r2, r3, r4 := exactMRR(t, reports["r1"]), exactMRR(t, reports["r2"]), exactMRR(t, reports["r3"]), exactMRR(t, reports["r4"])
	if !r1.WithinTolerance || r1.Tolerance != mrrTolerance || len(r1.Losses) != 1 || r1.Losses[0] != (MRRLoss{"falling", 7, 8}) || !(*r1.Value < *r1.PreviousValue) {
		t.Fatalf("r1 has no tolerated fall: %+v", r1.MRRComparison)
	}
	for name, n := range map[string]NonRegression{"r2": r2, "r3": r3, "r4": r4} {
		if !(*n.HighWater > *n.Previous) || n.Tolerance != mrrTolerance {
			t.Fatalf("%s carries no watermark: %+v", name, n.MRRComparison)
		}
	}
	if r2.Revision == nil || r3.Revision != nil || r4.Revision != nil {
		t.Fatal("only r2 compares a revised fixture")
	}
	for _, n := range reports["r4"].Cohorts["development_period"].NonRegression {
		if n.PreviousValue == nil {
			t.Fatalf("r4 period record has no previous value: %+v", n)
		}
	}
	if err = os.WriteFile(filepath.Join(dir, "fixture-1.json"), previousFixture, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "fixture-2.json"), currentFixture, 0600); err != nil {
		t.Fatal(err)
	}
	writeReleaseFile(t, filepath.Join(dir, "pairs.json"), [][2]string{{"r1", "r0"}, {"r2", "r1"}, {"r3", "r1"}, {"r4", "r2"}})
	writeReleaseFile(t, filepath.Join(dir, "release.json"), releaseRow(t, engine("20990102_1", 8).Release()))
	command := exec.CommandContext(ctx, "python3", "-c", "import sys; from corpus_revision_support import validate_go_reports; print(validate_go_reports(sys.argv[1]))", dir)
	command.Dir = filepath.Join("..", "..", "..", "..", "deploy", "github")
	command.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	data, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("Python release validator refused Go reports: %v: %s", err, data)
	}
	if got := string(data); got != "r1 r2 r3 r4\n" {
		t.Fatalf("Python release validator output %q", got)
	}
}
