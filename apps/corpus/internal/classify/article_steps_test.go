package classify

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
	"github.com/anthropics/anthropic-sdk-go/v2"
)

func stepRecipe(t *testing.T, step int) Recipe {
	t.Helper()
	r := articleRecipe(t)
	b, err := json.Marshal(r.Article)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err = json.Unmarshal(b, &fields); err != nil {
		t.Fatal(err)
	}
	fields["prompt_version"] = 3
	fields["step"] = step
	if step == 2 {
		fields["step_1_sha256"] = strings.Repeat("c", 64)
	}
	b, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, r.Article); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestArticleSteps_FamilyRequestAndValidation(t *testing.T) {
	r := stepRecipe(t, 1)
	plan, err := BuildArticlePlan(context.Background(), r, articleInputs(2, "chapter"), map[string]Outcome{"vn:vbpl:1": {Outcome: "ok", Codes: []string{"1.1"}}}, articleNodes(), func(context.Context, anthropic.MessageNewParams) (int64, error) { return 100, nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Requests) != 1 || len(plan.Requests[0].Nodes) != 1 || plan.Requests[0].Nodes[0].Code != "1.1" {
		t.Fatalf("plan %+v", plan)
	}
	p, err := BuildArticleRequest(r, plan.Requests[0])
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(p)
	if strings.Contains(string(b), "Child") || !strings.Contains(string(b), "level-2") {
		t.Fatalf("payload %s", b)
	}
	out, _ := ValidateArticles("end_turn", `{"dieu-1":{"codes":[{"code":"1.1", "confidence":0.7},{"code":"1.1.c1","confidence":0.9},{"code":"2.1","confidence":0.8}]},"dieu-2":{"codes":[]}}`, plan.Requests[0], articleNodes())
	if len(out["dieu-1"].Codes) != 1 || len(out["dieu-1"].Dropped) != 2 {
		t.Fatalf("outcomes %+v", out)
	}
}

func TestArticleSteps_RecipeSeparatesStepsAndLegacy(t *testing.T) {
	legacy := articleRecipe(t)
	first, second := stepRecipe(t, 1), stepRecipe(t, 2)
	if err := first.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := second.Validate(); err != nil {
		t.Fatal(err)
	}
	if first.Hash() == legacy.Hash() || second.Hash() == first.Hash() {
		t.Fatal("step recipes share a hash")
	}
	second.Model = "claude-sonnet-5-5"
	second.Article.SourceRecipeHash = second.documentRecipe().Hash()
	if second.Validate() == nil {
		t.Fatal("accepted a Sonnet trial")
	}
}

func TestArticleSteps_MergeFamilyResults(t *testing.T) {
	for _, tc := range []struct {
		name          string
		outcomes      []string
		want          string
		codes, failed int
	}{
		{"full success", []string{"ok", "ok"}, "ok", 2, 0},
		{"partial failure", []string{"ok", "invalid_response"}, "ok", 1, 1},
		{"total failure", []string{"invalid_response", "invalid_response"}, "invalid_response", 0, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := stepRecipe(t, 2)
			s := Submission{Recipe: r, Country: "vn", Release: "20990101", Settled: true, Results: map[string]Outcome{}}
			for i, status := range tc.outcomes {
				key := []string{"first", "second"}[i]
				out := emptyArticle(status)
				if status == "ok" {
					out.Codes = []CodeConfidence{{[]string{"1.1.c1", "2.1"}[i], 0.8}}
				}
				s.Items = append(s.Items, Item{Key: key, Article: &ArticleRequest{DocumentKey: "vn:vbpl:1", Articles: articleInputs(1, "")}})
				s.Results[key] = Outcome{Articles: map[string]ArticleOutcome{"dieu-1": out}}
			}
			dir := t.TempDir()
			if err := writeArticles(dir, s); err != nil {
				t.Fatal(err)
			}
			var a struct {
				Articles []struct {
					Result struct {
						Outcome string           `json:"outcome"`
						Codes   []CodeConfidence `json:"codes"`
						Failed  *int             `json:"failed_families"`
					} `json:"result"`
				} `json:"articles"`
			}
			if _, err := readArtifact(filepath.Join(dir, "articles.json"), &a); err != nil {
				t.Fatal(err)
			}
			if len(a.Articles) != 1 {
				t.Fatalf("articles %+v", a)
			}
			out := a.Articles[0].Result
			if out.Outcome != tc.want || len(out.Codes) != tc.codes || out.Failed == nil || *out.Failed != tc.failed {
				t.Fatalf("result %+v", out)
			}
		})
	}
}

func TestArticleSteps_NodePlanUsesChosenFamilyAndChapter(t *testing.T) {
	r := stepRecipe(t, 2)
	articles := articleInputs(3, "chapter")
	for i := range articles {
		articles[i].ChapterHeading = "Document chapter heading"
	}
	choices := map[string]Outcome{
		"vn:vbpl:1/dieu-1": {Outcome: "ok", Codes: []string{"1.1", "2.1"}},
		"vn:vbpl:1/dieu-2": {Outcome: "ok", Codes: []string{"1.1"}},
		"vn:vbpl:1/dieu-3": {Outcome: "invalid_response"},
	}
	plan, err := BuildArticlePlan(context.Background(), r, articles, choices, articleNodes(), func(context.Context, anthropic.MessageNewParams) (int64, error) { return 100, nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Requests) != 2 || len(plan.Skipped) != 1 || len(plan.Requests[0].Articles) != 2 || len(plan.Requests[1].Articles) != 1 {
		t.Fatalf("plan %+v", plan)
	}
	for _, req := range plan.Requests {
		if req.Step != 2 || req.Family == "" {
			t.Fatalf("request %+v", req)
		}
		p, buildErr := BuildArticleRequest(r, req)
		if buildErr != nil {
			t.Fatal(buildErr)
		}
		b, _ := json.Marshal(p)
		if !strings.Contains(string(b), "Document chapter heading") {
			t.Fatalf("missing chapter: %s", b)
		}
		for _, n := range req.Nodes {
			if n.Level == 2 && n.Code != req.Family {
				t.Fatalf("foreign family %+v", req)
			}
		}
	}
	bad := plan.Requests[0]
	bad.Nodes = append(append([]Node(nil), bad.Nodes...), articleNodes()[5])
	if _, err = BuildArticleRequest(r, bad); err == nil {
		t.Fatal("accepted multiple subtrees")
	}
	out, _ := ValidateArticles("end_turn", `{"dieu-1":{"codes":[{"code":"1.1.c1","confidence":0.8},{"code":"2.1","confidence":0.9}]},"dieu-2":{"codes":[]}}`, plan.Requests[0], articleNodes())
	if len(out["dieu-1"].Codes) != 1 || len(out["dieu-1"].Dropped) != 1 {
		t.Fatalf("outcomes %+v", out)
	}
}

func TestArticleSteps_RefusesMissingStep1BeforeCounting(t *testing.T) {
	runner, ledger, client, o, families := runArticlesFixture(t)
	o.Recipe = stepRecipe(t, 2)
	o.DryRun = true
	_, err := runner.Articles(context.Background(), o, "labels", articleInputs(1, ""), families, articleNodes())
	if err == nil || !strings.Contains(err.Error(), "step-1 artifact") || client.counts != 0 || len(ledger.rows) != 0 {
		t.Fatalf("error %v counts %d rows %d", err, client.counts, len(ledger.rows))
	}
}

func TestArticleSteps_SelectsOwnChapterHeading(t *testing.T) {
	provisions := []ProvisionEvidence{
		{DocumentKey: "vn:vbpl:1", Kind: "chuong", Path: "n1", Heading: "Own chapter"},
		{DocumentKey: "vn:vbpl:1", Kind: "dieu", Path: "n1.n1", Anchor: "dieu-1"},
		{DocumentKey: "vn:vbpl:1", Kind: "chuong", Path: "n2", Heading: "Other chapter"},
	}
	articles, err := SelectArticles(provisions, []taxonomy.LabelArticle{{Key: "vn:vbpl:1", Anchor: "dieu-1"}})
	if err != nil || len(articles) != 1 || articles[0].ChapterHeading != "Own chapter" {
		t.Fatalf("articles %+v error %v", articles, err)
	}
}

func TestArticleSteps_NodeGroupsCountOnlyChosenArticles(t *testing.T) {
	r := stepRecipe(t, 2)
	articles := articleInputs(25, "")
	choices := map[string]Outcome{}
	for i, a := range articles {
		out := Outcome{Outcome: "ok"}
		if i%2 == 0 {
			out.Codes = []string{"1.1"}
		}
		choices[a.DocumentKey+"/"+a.Anchor] = out
	}
	plan, err := BuildArticlePlan(context.Background(), r, articles, choices, articleNodes(), func(context.Context, anthropic.MessageNewParams) (int64, error) { return 100, nil })
	if err != nil || len(plan.Requests) != 2 || len(plan.Requests[0].Articles) != 12 || len(plan.Requests[1].Articles) != 1 || len(plan.Skipped) != 12 {
		t.Fatalf("plan %+v error %v", plan, err)
	}
}

func TestArticleSteps_ValidationRestrictsEachStep(t *testing.T) {
	for _, step := range []int{1, 2} {
		req := ArticleRequest{Step: step, Family: "1.1", Articles: articleInputs(1, ""), Nodes: articleNodes()}
		if step == 1 {
			req.Nodes = req.Nodes[:4]
		}
		text := `{"dieu-1":{"codes":[{"code":"1.1","confidence":0.8},{"code":"1.1.c1","confidence":0.9},{"code":"2.1","confidence":0.7}]}}`
		out, _ := ValidateArticles("end_turn", text, req, articleNodes())
		want := 1
		if step == 2 {
			want = 2
		}
		if len(out["dieu-1"].Codes) != want {
			t.Fatalf("step %d outcomes %+v", step, out)
		}
	}
}
