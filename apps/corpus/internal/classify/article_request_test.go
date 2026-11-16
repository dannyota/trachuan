package classify

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

func articleRecipe(t *testing.T) Recipe {
	t.Helper()
	base, err := NewRecipe("claude-haiku-5-5", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewArticleRecipe(base, strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func articleNodes() []Node {
	return []Node{{Code: "1", Level: 1, Active: true, Origin: "phapdien"}, {Code: "1.1", Name: "Family", Parent: "1", Level: 2, Active: true, Origin: "phapdien"}, {Code: "1.1.c1", Name: "Child", Parent: "1.1", Level: 3, Active: true, Origin: "phapdien"}, {Code: "1.1.x1", Name: "Custom", Parent: "1.1", Level: 3, Active: true, Origin: "custom"}, {Code: "1.1.old", Parent: "1.1", Level: 3, Origin: "phapdien"}, {Code: "2.1", Level: 2, Active: true, Origin: "phapdien"}}
}

func articleInputs(n int, chapter string) []Article {
	out := make([]Article, n)
	for i := range out {
		out[i] = Article{DocumentKey: "vn:vbpl:1", Anchor: fmt.Sprintf("dieu-%d", i+1), Chapter: chapter, Text: "Synthetic public legal text", OwnText: "Synthetic public legal text"}
	}
	return out
}

func TestArticleRequests_GroupSplitAndBound(t *testing.T) {
	r := articleRecipe(t)
	count := func(_ context.Context, p anthropic.MessageNewParams) (int64, error) {
		b, _ := json.Marshal(p)
		return int64(strings.Count(string(b), `\"anchor\"`)*100 + strings.Count(string(b), "Child")*50), nil
	}
	f := map[string]Outcome{"vn:vbpl:1": {Outcome: "ok", Codes: []string{"1.1"}}}
	for _, tc := range []struct {
		chapter string
		bound   int64
		sizes   []int
	}{{"", 90000, []int{12, 1}}, {"chapter-1", 90000, []int{13}}, {"chapter-1", 350, []int{3, 3, 3, 3, 1}}} {
		r.InputBound = tc.bound
		r.Article.SourceRecipeHash = r.documentRecipe().Hash()
		plan, err := BuildArticlePlan(context.Background(), r, articleInputs(13, tc.chapter), f, articleNodes(), count)
		if err != nil {
			t.Fatal(err)
		}
		if len(plan.Requests) != len(tc.sizes) {
			t.Fatalf("requests %d want %v", len(plan.Requests), tc.sizes)
		}
		for i, req := range plan.Requests {
			if len(req.Articles) != tc.sizes[i] || len(req.Nodes) != 3 {
				t.Fatalf("request %+v", req)
			}
			p, buildErr := BuildArticleRequest(r, req)
			if buildErr != nil {
				t.Fatal(buildErr)
			}
			b, _ := json.Marshal(p)
			for _, forbidden := range []string{"maxItems", "minimum", "maximum", "minLength", "own_text", "seed"} {
				if strings.Contains(string(b), forbidden) {
					t.Fatalf("request contains %s", forbidden)
				}
			}
			if strings.Count(string(b), "Child") != 1 {
				t.Fatal("subtree not included once")
			}
			if p.MaxTokens != r.Article.MaxTokens {
				t.Fatal("wrong article output bound")
			}
		}
	}
}

func TestArticleRequests_TooLongAndFailedFamilies(t *testing.T) {
	r := articleRecipe(t)
	count := func(context.Context, anthropic.MessageNewParams) (int64, error) { return r.InputBound + 1, nil }
	plan, err := BuildArticlePlan(context.Background(), r, articleInputs(2, "c"), map[string]Outcome{"vn:vbpl:1": {Outcome: "ok", Codes: []string{"1.1"}}}, articleNodes(), count)
	if err != nil || len(plan.Requests) != 0 || len(plan.Skipped) != 2 || plan.Skipped[0].Outcome.Outcome != "too_long" {
		t.Fatalf("plan %+v error %v", plan, err)
	}
	for _, out := range []Outcome{{Outcome: "invalid_response"}, {Outcome: "no_family"}} {
		plan, err = BuildArticlePlan(context.Background(), r, articleInputs(2, ""), map[string]Outcome{"vn:vbpl:1": out}, articleNodes(), func(context.Context, anthropic.MessageNewParams) (int64, error) {
			t.Fatal("counted skipped request")
			return 0, nil
		})
		if err != nil || len(plan.Skipped) != 2 || plan.Skipped[0].Outcome.Outcome != out.Outcome {
			t.Fatalf("plan %+v error %v", plan, err)
		}
	}
}

func TestArticleRecipe_BindsEveryVersionAndOutputBound(t *testing.T) {
	r := articleRecipe(t)
	for _, change := range []func(*Recipe){func(v *Recipe) { v.Article.PromptVersion++ }, func(v *Recipe) { v.Article.SchemaVersion++ }, func(v *Recipe) { v.Article.MaxTokens++ }, func(v *Recipe) { v.Article.FamiliesHash = strings.Repeat("c", 64) }} {
		v := r
		a := *r.Article
		v.Article = &a
		change(&v)
		if v.Hash() == r.Hash() {
			t.Fatal("recipe hash unchanged")
		}
	}
	reserve, err := r.Reserve(100)
	if err != nil || reserve != r.cost(100, r.Article.MaxTokens) {
		t.Fatalf("reserve %d error %v", reserve, err)
	}
}
