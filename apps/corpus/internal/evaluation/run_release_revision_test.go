package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	rel string
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
	s := periodSuite()
	s.Hashes.OldProduct = "old-product-fixture"
	s.Development = cases
	for _, c := range cases {
		if len(c.ExpectedCitations) > 0 && c.ID != "development-positive" && c.ID != "period-positive" && c.ID != "transition-positive" {
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
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseValidator_AcceptsRevisedVersionThreeReports(t *testing.T) {
	ctx := context.Background()
	date := retrieval.Date{Year: 2099, Month: 1, Day: 2}
	engine := func(rel string) *releaseEngine {
		return &releaseEngine{periodEngine: periodEngine{recordedEngine: recordedEngine{fakeEngine: fakeEngine{date: date}}}, rel: rel}
	}
	base := periodSuite().Development
	base = append(base, Case{ID: "moving", Question: "Câu hỏi tốt", ExpectedCitations: []Expectation{{DocNumber: "doc"}}, Tags: []string{"multi_document"}})
	previousSuite, previousFixture := releaseSuite(t, base)
	revised := append([]Case(nil), base...)
	revised[0].Tags = []string{"identifier", "transition"}
	revised[len(revised)-1].Tags = []string{"historical", "multi_document"}
	revised = append(revised, Case{ID: "added", Question: "Câu hỏi tốt", ExpectedCitations: []Expectation{{DocNumber: "doc"}}, Tags: []string{"clause"}})
	currentSuite, currentFixture := releaseSuite(t, revised)
	currentSuite.PreviousDevelopment = map[string][]byte{previousSuite.Hashes.Development: previousFixture}

	dir := t.TempDir()
	previous, _, err := Run(ctx, engine("20990101_1"), previousSuite, Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	if err = previous.WriteFiles(filepath.Join(dir, "base")); err != nil {
		t.Fatal(err)
	}
	options := Options{EvaluationDate: date, PreviousReport: filepath.Join(dir, "base", "report.json")}
	for name, suite := range map[string]Suite{"compared": previousSuite, "revised": currentSuite} {
		r, _, runErr := Run(ctx, engine("20990102_1"), suite, options)
		if runErr != nil {
			t.Fatal(runErr)
		}
		if r.Version != 3 || !r.QualityPass {
			t.Fatalf("%s: version %d quality %v", name, r.Version, r.QualityPass)
		}
		if err = r.WriteFiles(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err = os.WriteFile(filepath.Join(dir, "fixture-1.json"), previousFixture, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "fixture-2.json"), currentFixture, 0600); err != nil {
		t.Fatal(err)
	}
	writeReleaseFile(t, filepath.Join(dir, "release.json"), releaseRow(t, engine("20990102_1").Release()))
	command := exec.CommandContext(ctx, "python3", "-c", "import sys; from corpus_revision_support import validate_go_reports; print(validate_go_reports(sys.argv[1]))", dir)
	command.Dir = filepath.Join("..", "..", "..", "..", "deploy", "github")
	command.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	data, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("Python release validator refused Go reports: %v: %s", err, data)
	}
	if got := string(data); got != "compared revised\n" {
		t.Fatalf("Python release validator output %q", got)
	}
}
