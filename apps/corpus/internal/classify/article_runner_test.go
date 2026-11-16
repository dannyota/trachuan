package classify

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestArticles_RefusesIncompleteFamiliesBeforeCountingOrReplay(t *testing.T) {
	for _, replay := range []bool{false, true} {
		t.Run(fmt.Sprint(replay), func(t *testing.T) {
			r, ledger, client, o, families := runArticlesFixture(t)
			articles := append(articleInputs(1, ""), Article{DocumentKey: "vn:vbpl:2", Anchor: "dieu-1", Text: "Synthetic", OwnText: "Synthetic"})
			o.DryRun = true
			if replay {
				hashed := o
				hashed.Documents = 1
				hashed.article = &articleWork{articles: articles, families: families, nodes: articleNodes(), labelsHash: "labels"}
				ledger.rows = []Submission{{ScreenHash: screenHash(hashed), State: "completed"}}
			}
			_, err := r.Articles(context.Background(), o, "labels", articles, families, articleNodes())
			if err == nil || !strings.Contains(err.Error(), "missing from families") || client.counts != 0 || client.creates != 0 {
				t.Fatalf("error %v, counts %d creates %d", err, client.counts, client.creates)
			}
		})
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

func TestArticles_DropsForeignAnchorsInOutcomeAndArtifact(t *testing.T) {
	r, ledger, client, options, families := runArticlesFixture(t)
	prepare := client.beforeCreate
	client.beforeCreate = func(requests []batchapi.Request) {
		prepare(requests)
		block := &client.results[0].Message.Content[0]
		block.Text = strings.TrimSuffix(block.Text, "}") + `,"foreign":null}`
	}
	if _, err := r.Articles(context.Background(), options, "labels", articleInputs(1, ""), families, articleNodes()); err != nil {
		t.Fatal(err)
	}
	for _, outcome := range ledger.rows[0].Results {
		b, err := json.Marshal(outcome)
		if err != nil {
			t.Fatal(err)
		}
		var recorded struct {
			DroppedAnchors int `json:"dropped_anchors"`
		}
		if err = json.Unmarshal(b, &recorded); err != nil {
			t.Fatal(err)
		}
		if recorded.DroppedAnchors != 1 || outcome.Articles["dieu-1"].Outcome != "ok" {
			t.Fatalf("outcome did not preserve result and count: %s", b)
		}
	}
	b, err := os.ReadFile(filepath.Join(options.Out, "articles.json"))
	if err != nil {
		t.Fatal(err)
	}
	var artifact struct {
		DroppedAnchors int             `json:"dropped_anchors"`
		Articles       []ArticleRecord `json:"articles"`
	}
	if err = json.Unmarshal(b, &artifact); err != nil {
		t.Fatal(err)
	}
	if artifact.DroppedAnchors != 1 || len(artifact.Articles) != 1 || len(artifact.Articles[0].Outcome.Codes) != 1 {
		t.Fatalf("artifact did not preserve result and count: %s", b)
	}
}
