package classify

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/batchapi"
	"github.com/anthropics/anthropic-sdk-go/v2"
)

func runArticlesFixture(t *testing.T) (*Runner, *memoryLedger, *fakeClient, Options, FamilyArtifact) {
	t.Helper()
	r, m, f, o := screenFixture(t)
	o.Recipe = articleRecipe(t)
	f.beforeCreate = func(requests []batchapi.Request) {
		s := m.rows[0]
		if _, err := os.Stat(filepath.Join(r.IntentDir, "submissions", s.ID+".json")); err != nil {
			t.Fatal(err)
		}
		if requests[0].CustomID != customID(s.ID, 0) {
			t.Fatal("wrong custom id")
		}
		var message anthropic.Message
		if err := message.UnmarshalJSON([]byte(`{"stop_reason":"end_turn","content":[{"type":"text","text":"{\"dieu-1\":{\"codes\":[{\"code\":\"1.1.c1\",\"confidence\":0.9}]}}"}],"usage":{"input_tokens":100,"output_tokens":5000}}`)); err != nil {
			t.Fatal(err)
		}
		f.results = []batchapi.Result{{CustomID: requests[0].CustomID, Type: "succeeded", Message: message}}
	}
	base := o.Recipe.documentRecipe()
	return r, m, f, o, FamilyArtifact{Version: 1, Recipe: base, RecipeHash: base.Hash(), TaxonomyHash: base.TaxonomyHash, Documents: map[string]Outcome{"vn:vbpl:1": {Outcome: "ok", Codes: []string{"1.1"}}}}
}

func TestArticles_ReusesLedgerAndResumesWithoutCounting(t *testing.T) {
	r, m, f, o, families := runArticlesFixture(t)
	if _, err := r.Articles(context.Background(), o, "labels", articleInputs(1, ""), families, articleNodes()); err != nil {
		t.Fatal(err)
	}
	if !m.rows[0].Settled || m.rows[0].CostMicroUSD != 1255 {
		t.Fatalf("ledger %+v", m.rows)
	}
	before := f.counts + f.creates + f.gets + f.downloads
	if _, err := r.Articles(context.Background(), o, "labels", articleInputs(1, ""), families, articleNodes()); err != nil {
		t.Fatal(err)
	}
	if f.counts+f.creates+f.gets+f.downloads != before {
		t.Fatal("resume called provider")
	}
	var artifact ArticleArtifact
	b, err := os.ReadFile(filepath.Join(o.Out, "articles.json"))
	if err != nil || json.Unmarshal(b, &artifact) != nil || len(artifact.Articles) != 1 || artifact.Articles[0].Outcome.Codes[0].Code != "1.1.c1" {
		t.Fatalf("artifact %s error %v", b, err)
	}
}

func TestArticles_DryRunAndAdmission(t *testing.T) {
	r, m, f, o, families := runArticlesFixture(t)
	o.DryRun = true
	s, err := r.Articles(context.Background(), o, "labels", articleInputs(1, ""), families, articleNodes())
	if err != nil || s.Requests != 1 || s.InputTokens != 100 || s.ReserveMicroUSD != 2053 || !s.Admitted || len(m.rows) != 0 || f.creates != 0 {
		t.Fatalf("summary %+v error %v", s, err)
	}
	m.rows = []Submission{{RunID: 2, State: "failed", ReserveMicroUSD: 15000000}}
	o.DryRun = false
	if _, err = r.Articles(context.Background(), o, "labels", articleInputs(1, ""), families, articleNodes()); err == nil || f.creates != 0 {
		t.Fatal("overspent budget")
	}
}

func TestArticles_AllSkippedWritesWithoutSubmission(t *testing.T) {
	r, m, f, o, families := runArticlesFixture(t)
	families.Documents["vn:vbpl:1"] = Outcome{Outcome: "no_family"}
	s, err := r.Articles(context.Background(), o, "labels", articleInputs(1, ""), families, articleNodes())
	if err != nil || s.Requests != 0 || len(m.rows) != 0 || f.counts != 0 || f.creates != 0 {
		t.Fatalf("summary %+v error %v", s, err)
	}
	if _, err = os.Stat(filepath.Join(o.Out, "articles.json")); err != nil {
		t.Fatal(err)
	}
}
