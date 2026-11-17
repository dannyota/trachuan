package classify

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/batchapi"
	"github.com/anthropics/anthropic-sdk-go/v2"
)

func step1Fixture(t *testing.T, o Options, labelsHash string, articles []Article) (ArticleArtifact, string, string) {
	t.Helper()
	r := stepRecipe(t, 1)
	a := ArticleArtifact{Version: 1, Country: o.Country, Release: o.Release, Recipe: r, RecipeHash: r.Hash(), TaxonomyHash: r.TaxonomyHash, FamiliesHash: r.Article.FamiliesHash, LabelsHash: labelsHash, Settled: true, InputHash: articleInputHash(o.Country, o.Release, labelsHash, articles)}
	for _, article := range articles {
		a.Articles = append(a.Articles, ArticleRecord{article.DocumentKey, article.Anchor, ArticleOutcome{Outcome: "ok", Codes: []CodeConfidence{{"1.1", 0.8}, {"2.1", 0.7}}}})
	}
	path, hash := publishStep1(t, a)
	return a, path, hash
}

func publishStep1(t *testing.T, a ArticleArtifact) (string, string) {
	t.Helper()
	dir := t.TempDir()
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if err = writeOnce(filepath.Join(dir, "articles.json"), b); err != nil {
		t.Fatal(err)
	}
	if err = writeOnce(filepath.Join(dir, "MANIFEST.sha256"), []byte(digest(b)+"  articles.json\n")); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "articles.json"), digest(b)
}

func TestArticleSteps_RefusesInvalidStep1Artifacts(t *testing.T) {
	for _, reason := range []string{"manifest", "recipe hash", "unsettled", "wrong step", "wrong source", "release", "labels", "inputs", "missing article", "duplicate article", "foreign article", "foreign family", "deep code", "bound hash", "failed families"} {
		t.Run(reason, func(t *testing.T) {
			runner, ledger, client, o, families := runArticlesFixture(t)
			o.Recipe = stepRecipe(t, 2)
			o.DryRun = true
			articles := articleInputs(1, "chapter")
			a, _, _ := step1Fixture(t, o, "labels", articles)
			switch reason {
			case "recipe hash":
				a.RecipeHash = strings.Repeat("f", 64)
			case "unsettled":
				a.Settled = false
			case "wrong step":
				a.Recipe = stepRecipe(t, 2)
				a.RecipeHash = a.Recipe.Hash()
			case "wrong source":
				a.Recipe.Effort = "low"
				a.Recipe.Article.SourceRecipeHash = a.Recipe.documentRecipe().Hash()
				a.RecipeHash = a.Recipe.Hash()
			case "release":
				a.Release = "20990102"
			case "labels":
				a.LabelsHash = "different"
			case "inputs":
				a.InputHash = strings.Repeat("d", 64)
			case "missing article":
				a.Articles = nil
			case "duplicate article":
				a.Articles = append(a.Articles, a.Articles[0])
			case "foreign article":
				a.Articles[0].Anchor = "foreign"
			case "foreign family":
				a.Articles[0].Outcome.Codes = []CodeConfidence{{"2.1", 0.8}}
			case "deep code":
				a.Articles[0].Outcome.Codes = []CodeConfidence{{"1.1.c1", 0.8}}
			case "failed families":
				failed := 1
				a.Articles[0].Outcome.FailedFamilies = &failed
			}
			if reason != "foreign family" {
				families.Documents["vn:vbpl:1"] = Outcome{Outcome: "ok", Codes: []string{"1.1", "2.1"}}
			}
			path, hash := publishStep1(t, a)
			o.Step1Artifact = path
			o.Recipe.Article.Step1Hash = hash
			if reason == "bound hash" {
				o.Recipe.Article.Step1Hash = strings.Repeat("f", 64)
			}
			if reason == "manifest" {
				if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			_, err := runner.Articles(context.Background(), o, "labels", articles, families, articleNodes())
			if err == nil || client.counts != 0 || client.creates != 0 || len(ledger.rows) != 0 {
				t.Fatalf("error %v counts %d creates %d rows %d", err, client.counts, client.creates, len(ledger.rows))
			}
		})
	}
}

func TestArticleSteps_RunnerDryRunSubmissionReplayAndRecovery(t *testing.T) {
	runner, ledger, client, o, families := runArticlesFixture(t)
	o.Recipe = stepRecipe(t, 2)
	articles := articleInputs(1, "chapter")
	_, path, hash := step1Fixture(t, o, "labels", articles)
	o.Step1Artifact = path
	o.Recipe.Article.Step1Hash = hash
	families.Documents["vn:vbpl:1"] = Outcome{Outcome: "ok", Codes: []string{"1.1", "2.1"}}
	o.DryRun = true
	summary, err := runner.Articles(context.Background(), o, "labels", articles, families, articleNodes())
	if err != nil || summary.Requests != 2 || summary.InputTokens != 200 || summary.ReserveMicroUSD != 4106 || len(ledger.rows) != 0 || client.creates != 0 {
		t.Fatalf("summary %+v error %v", summary, err)
	}
	client.beforeCreate = func(requests []batchapi.Request) {
		if len(ledger.rows) != 1 || ledger.rows[0].ItemsHash == "" {
			t.Fatal("missing durable ledger")
		}
		for i, request := range requests {
			var message anthropic.Message
			if err = message.UnmarshalJSON([]byte(`{"stop_reason":"end_turn","content":[{"type":"text","text":"{\"dieu-1\":{\"codes\":[{\"code\":\"1.1.c1\",\"confidence\":0.9}]}}"}],"usage":{"input_tokens":100,"output_tokens":100}}`)); err != nil {
				t.Fatal(err)
			}
			typ := "succeeded"
			if i == 1 {
				typ = "errored"
			}
			client.results = append(client.results, batchapi.Result{CustomID: request.CustomID, Type: typ, Message: message})
		}
	}
	o.DryRun = false
	if _, err = runner.Articles(context.Background(), o, "labels", articles, families, articleNodes()); err != nil {
		t.Fatal(err)
	}
	if len(ledger.rows) != 1 || !ledger.rows[0].Settled || ledger.rows[0].Items[0].Key == ledger.rows[0].Items[1].Key {
		t.Fatalf("ledger %+v", ledger.rows)
	}
	a, err := ReadArticleArtifact(filepath.Join(o.Out, "articles.json"), o.Recipe.TaxonomyHash, o.Recipe.Article.FamiliesHash)
	if err != nil || len(a.Articles) != 1 || a.Articles[0].Outcome.FailedFamilies == nil || *a.Articles[0].Outcome.FailedFamilies != 1 || a.Articles[0].Outcome.Outcome != "ok" {
		t.Fatalf("artifact %+v error %v", a, err)
	}
	before := client.counts + client.creates + client.gets + client.downloads
	if _, err = runner.Articles(context.Background(), o, "labels", articles, families, articleNodes()); err != nil {
		t.Fatal(err)
	}
	if client.counts+client.creates+client.gets+client.downloads != before {
		t.Fatal("replay called provider")
	}
	ledger.rows = nil
	if err = runner.Rebuild(context.Background()); err != nil {
		t.Fatal(err)
	}
	counts, creates := client.counts, client.creates
	if _, err = runner.Articles(context.Background(), o, "labels", articles, families, articleNodes()); err != nil {
		t.Fatal(err)
	}
	if client.counts != counts || client.creates != creates || len(ledger.rows) != 1 || !ledger.rows[0].Settled {
		t.Fatalf("recovery recounted or recreated: %+v", ledger.rows)
	}
}

func TestArticleSteps_Step1SettlesBeforeArtifact(t *testing.T) {
	runner, ledger, client, o, families := runArticlesFixture(t)
	o.Recipe = stepRecipe(t, 1)
	prepare := client.beforeCreate
	client.beforeCreate = func(requests []batchapi.Request) {
		prepare(requests)
		client.results[0].Message.Content[0].Text = `{"dieu-1":{"codes":[{"code":"1.1","confidence":0.8}]}}`
	}
	o.DryRun = true
	summary, err := runner.Articles(context.Background(), o, "labels", articleInputs(1, ""), families, articleNodes())
	if err != nil || summary.Step != 1 || summary.Requests != 1 || summary.InputTokens != 100 || summary.ReserveMicroUSD != 2053 || client.creates != 0 {
		t.Fatalf("summary %+v error %v", summary, err)
	}
	o.DryRun = false
	client.ended = false
	if _, err = runner.Articles(context.Background(), o, "labels", articleInputs(1, ""), families, articleNodes()); err == nil {
		t.Fatal("expected interrupted polling")
	}
	path := filepath.Join(o.Out, "articles.json")
	if _, _, err = ReadStep1Artifact(path, o.Recipe.TaxonomyHash, o.Recipe.Article.FamiliesHash); err == nil {
		t.Fatal("unsettled step 1 exposed an artifact")
	}
	client.ended = true
	if _, err = runner.Articles(context.Background(), o, "labels", articleInputs(1, ""), families, articleNodes()); err != nil {
		t.Fatal(err)
	}
	a, _, err := ReadStep1Artifact(path, o.Recipe.TaxonomyHash, o.Recipe.Article.FamiliesHash)
	if err != nil || !ledger.rows[0].Settled || a.Articles[0].Outcome.Codes[0].Code != "1.1" {
		t.Fatalf("artifact %+v error %v", a, err)
	}
}
