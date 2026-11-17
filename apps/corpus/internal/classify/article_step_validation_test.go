package classify

import "testing"

func TestArticleSteps_FinalArtifactRequiresValidFailureCounts(t *testing.T) {
	negative, one, zero := -1, 1, 0
	for _, out := range []ArticleOutcome{
		{Outcome: "ok"},
		{Outcome: "ok", FailedFamilies: &negative},
		{Outcome: "no_family", FailedFamilies: &one},
		{Outcome: "too_long", FailedFamilies: &zero},
	} {
		r := stepRecipe(t, 2)
		a := ArticleArtifact{Version: 1, Recipe: r, RecipeHash: r.Hash(), TaxonomyHash: r.TaxonomyHash, FamiliesHash: r.Article.FamiliesHash, Articles: []ArticleRecord{{DocumentKey: "vn:vbpl:1", Anchor: "dieu-1", Outcome: out}}}
		path, _ := publishStep1(t, a)
		if _, err := ReadArticleArtifact(path, r.TaxonomyHash, r.Article.FamiliesHash); err == nil {
			t.Fatalf("accepted outcome %+v", out)
		}
	}
}
