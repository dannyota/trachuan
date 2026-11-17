package classify

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

func TestArticleRecipe_DefaultIsMenuVersion(t *testing.T) {
	base, err := NewRecipe("claude-haiku-5-5", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewArticleRecipe(base, strings.Repeat("b", 64))
	if err != nil || r.Article.PromptVersion != 4 {
		t.Fatalf("recipe %+v error %v", r.Article, err)
	}
}

func TestArticleRequest_MenuPromptAndSchema(t *testing.T) {
	base, err := NewRecipe("claude-haiku-5-5", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewArticleRecipe(base, strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	r.Article.PromptVersion = 4
	req := ArticleRequest{DocumentKey: "vn:vbpl:1", Articles: articleInputs(1, "c"), Nodes: articleNodes()[1:2]}
	p, err := BuildArticleRequest(r, req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Assign a node only when", `"primary"`, `"additional"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("request missing %q", want)
		}
	}
	for _, forbidden := range []string{"maxItems", "minItems", "maximum", "minimum", "minLength", "maxLength"} {
		if strings.Contains(string(b), forbidden) {
			t.Fatalf("unsupported schema constraint %s", forbidden)
		}
	}
}

func TestArticlePlan_MenuIgnoresFailedDocumentRequest(t *testing.T) {
	r := articleRecipe(t)
	r.Article.PromptVersion = 4
	nodes := articleNodes()
	articles := articleInputs(1, "chapter")
	plan, err := BuildArticlePlan(context.Background(), r, articles, map[string]Outcome{articles[0].DocumentKey: {Outcome: "invalid_response"}}, nodes, func(context.Context, anthropic.MessageNewParams) (int64, error) { return 1, nil })
	if err != nil || len(plan.Requests) != 1 || len(plan.Skipped) != 0 {
		t.Fatalf("plan %+v error %v", plan, err)
	}
	if len(plan.Requests[0].DocumentCodes) != 0 {
		t.Fatalf("unexpected document context: %+v", plan.Requests[0])
	}
}

func TestArticlePlan_MenuAndContext(t *testing.T) {
	r := articleRecipe(t)
	r.Article.PromptVersion = 4
	nodes := articleNodes()
	nodes[0].Name = "Topic"
	nodes[5].Parent = "1"
	nodes[5].Name = "Empty"
	articles := articleInputs(1, "chapter")
	articles[0].ChapterHeading = "Chapter heading"
	articles[0].DocumentTitle = "Document title"
	plan, err := BuildArticlePlan(context.Background(), r, articles, map[string]Outcome{articles[0].DocumentKey: {Outcome: "ok", Codes: []string{"1.1"}}}, nodes, func(context.Context, anthropic.MessageNewParams) (int64, error) { return 1, nil })
	if err != nil || len(plan.Requests) != 1 {
		t.Fatalf("plan %+v error %v", plan, err)
	}
	req := plan.Requests[0]
	if len(req.Nodes) != 1 || req.Nodes[0].Name != "Topic > Family" || req.DocumentTitle != "Document title" || len(req.DocumentCodes) != 1 || req.DocumentCodes[0] != "1.1" || req.Articles[0].ChapterHeading != "Chapter heading" {
		t.Fatalf("request %+v", req)
	}
}

func TestArticlePlan_BoundCountsDocumentTitle(t *testing.T) {
	r := articleRecipe(t)
	r.Article.PromptVersion = 4
	nodes := articleNodes()
	articles := articleInputs(1, "chapter")
	articles[0].DocumentTitle = strings.Repeat("long title ", 100)
	count := func(_ context.Context, p anthropic.MessageNewParams) (int64, error) {
		b, err := json.Marshal(p)
		return int64(len(b)), err
	}
	families := map[string]Outcome{articles[0].DocumentKey: {Outcome: "ok", Codes: []string{"1.1"}}}
	plan, err := BuildArticlePlan(context.Background(), r, articles, families, nodes, count)
	if err != nil || len(plan.Requests) != 1 {
		t.Fatalf("plan %+v error %v", plan, err)
	}
	r.InputBound = plan.Requests[0].InputTokens - 1
	r.Article.SourceRecipeHash = r.documentRecipe().Hash()
	plan, err = BuildArticlePlan(context.Background(), r, articles, families, nodes, count)
	if err != nil || len(plan.Skipped) != 1 || plan.Skipped[0].Outcome.Outcome != "too_long" {
		t.Fatalf("plan %+v error %v", plan, err)
	}
}

func TestArticleMenu_MatchesDocumentMenuOrder(t *testing.T) {
	nodes := []Node{{Code: "1", Name: "Topic", Level: 1, Active: true}, {Code: "1.2", Name: "Second", Parent: "1", Level: 2, Active: true, HasProvisions: true, Ordinal: 2}, {Code: "1.1", Name: "First", Parent: "1", Level: 2, Active: true, HasProvisions: true, Ordinal: 1}}
	menu := fullArticleMenu(nodes)
	if len(menu) != 2 || menu[0].Code != "1.1" || menu[1].Code != "1.2" {
		t.Fatalf("menu %+v", menu)
	}
}

func TestValidateArticles_MenuFailures(t *testing.T) {
	req := ArticleRequest{PromptVersion: 4, Articles: articleInputs(2, ""), Nodes: []Node{articleNodes()[1]}}
	for _, text := range []string{`{"dieu-1":{"primary":{"code":"1.1","confidence":0.5},"additional":null}}`, `{"dieu-1":{"primary":{"code":"1.1","confidence":0.5},"additional":null},"dieu-2":{"primary":null,"additional":null}}`} {
		out, _ := ValidateArticles("end_turn", text, req, req.Nodes)
		if out["dieu-1"].Outcome != "invalid_response" || out["dieu-2"].Outcome != "invalid_response" {
			t.Fatalf("accepted %s: %+v", text, out)
		}
	}
	text := `{"dieu-1":{"primary":{"code":"1.1","confidence":0.5},"additional":{"code":"1.1","confidence":0.8}},"dieu-2":{"primary":{"code":"1.1","confidence":0.5},"additional":{"code":"bad","confidence":0.8}}}`
	out, _ := ValidateArticles("end_turn", text, req, req.Nodes)
	for _, v := range out {
		if v.Outcome != "ok" || len(v.Codes) != 1 || len(v.Dropped) != 1 {
			t.Fatalf("outcomes %+v", out)
		}
	}
	out, _ = ValidateArticles("max_tokens", text, req, req.Nodes)
	for _, v := range out {
		if v.Outcome != "invalid_response" {
			t.Fatalf("truncated %+v", out)
		}
	}
}

func TestValidateArticles_MenuPrimaryAndAdditional(t *testing.T) {
	req := ArticleRequest{PromptVersion: 4, Articles: articleInputs(2, ""), Nodes: []Node{articleNodes()[1], articleNodes()[5]}}
	text := `{"dieu-1":{"primary":{"code":"1.1","confidence":0.4},"additional":{"code":"2.1","confidence":0.9}},"dieu-2":{"primary":{"code":"unknown","confidence":0.9},"additional":null},"extra":{}}`
	out, dropped := ValidateArticles("end_turn", text, req, req.Nodes)
	if dropped != 1 || out["dieu-1"].Outcome != "ok" || len(out["dieu-1"].Codes) != 2 || out["dieu-1"].Codes[0].Code != "1.1" || out["dieu-2"].Outcome != "invalid_response" || len(out["dieu-2"].Dropped) != 1 {
		t.Fatalf("outcomes %+v dropped %d", out, dropped)
	}
}

func TestScore_MenuPrimaryOverridesHigherConfidenceAdditional(t *testing.T) {
	labels, families, artifact, release, nodes := scoreFixture(t, 1)
	nodes = append(nodes, Node{Code: "2.1", Level: 2, Active: true, Origin: "phapdien"})
	artifact.Recipe.Article.PromptVersion = 4
	artifact.Articles[0].Outcome.Codes = []CodeConfidence{{"1.1", 0.4}, {"2.1", 0.9}}
	families.Documents[release[0].DocumentKey] = Outcome{Outcome: "invalid_response"}
	artifact.Version = 1
	artifact.RecipeHash = artifact.Recipe.Hash()
	dir := t.TempDir()
	b, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "articles.json")
	if err = writeOnce(path, b); err != nil {
		t.Fatal(err)
	}
	if err = writeOnce(filepath.Join(dir, "MANIFEST.sha256"), []byte(digest(b)+"  articles.json\n")); err != nil {
		t.Fatal(err)
	}
	artifact, err = ReadArticleArtifactForScore(path, artifact.TaxonomyHash, artifact.FamiliesHash)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Score(labels, families, artifact, release, nodes)
	if err != nil {
		t.Fatal(err)
	}
	m := report.Levels[0].All.Raw
	if m.TopOnePrecision != 1 || m.AdditionalTotal != 1 || m.AdditionalCorrect != 0 || m.Recall != 1 {
		t.Fatalf("metrics %+v", m)
	}
}

func TestValidateArticles_UnknownEmptyCodeIsDropped(t *testing.T) {
	req := ArticleRequest{PromptVersion: 4, Articles: articleInputs(2, ""), Nodes: []Node{articleNodes()[1]}}
	text := `{"dieu-1":{"primary":{"code":"","confidence":0.8},"additional":null},"dieu-2":{"primary":{"code":"1.1","confidence":0.7},"additional":null}}`
	out, _ := ValidateArticles("end_turn", text, req, req.Nodes)
	if out["dieu-1"].Outcome != "invalid_response" || len(out["dieu-1"].Dropped) != 1 || out["dieu-2"].Outcome != "ok" {
		t.Fatalf("outcomes %+v", out)
	}
}
