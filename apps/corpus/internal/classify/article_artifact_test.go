package classify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFamilyArtifact_VerifiesManifestRecipeAndTaxonomy(t *testing.T) {
	r := articleRecipe(t).documentRecipe()
	dir := t.TempDir()
	if err := writeFamilies(dir, Submission{Recipe: r, Results: map[string]Outcome{"vn:vbpl:1": {Outcome: "ok", Codes: []string{"1.1"}}}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "families.json")
	f, hash, err := ReadFamilyArtifact(path, r.TaxonomyHash, articleNodes())
	if err != nil || f.RecipeHash != r.Hash() || len(hash) != 64 {
		t.Fatalf("artifact %+v hash %s error %v", f, hash, err)
	}
	if _, _, err = ReadFamilyArtifact(path, "wrong", articleNodes()); err == nil {
		t.Fatal("accepted taxonomy mismatch")
	}
	if err = os.WriteFile(path, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = ReadFamilyArtifact(path, r.TaxonomyHash, articleNodes()); err == nil {
		t.Fatal("accepted tampering")
	}
}

func TestArticleArtifact_RejectsMalformedOutcomes(t *testing.T) {
	r := articleRecipe(t)
	for _, out := range []ArticleOutcome{{Outcome: "unknown"}, {Outcome: "too_long", Codes: []CodeConfidence{{"1.1", 0.9}}}, {Outcome: "ok", Codes: []CodeConfidence{{"1.1", 1.1}}}, {Outcome: "ok", Codes: []CodeConfidence{{"1.1", 0.9}, {"1.1", 0.8}}}} {
		dir := t.TempDir()
		a := ArticleArtifact{Version: 1, Recipe: r, RecipeHash: r.Hash(), TaxonomyHash: r.TaxonomyHash, FamiliesHash: r.Article.FamiliesHash, Articles: []ArticleRecord{{DocumentKey: "vn:vbpl:1", Anchor: "dieu-1", Outcome: out}}}
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
		if _, err = ReadArticleArtifact(filepath.Join(dir, "articles.json"), r.TaxonomyHash, r.Article.FamiliesHash); err == nil {
			t.Fatalf("accepted malformed outcome %+v", out)
		}
	}
}

func TestArticleArtifact_BindsInputsAndRejectsTampering(t *testing.T) {
	r := articleRecipe(t)
	s := Submission{Recipe: r, Country: "vn", Release: "20990101", Items: []Item{{Key: "request", Article: &ArticleRequest{DocumentKey: "vn:vbpl:1", Articles: articleInputs(1, "")}, ArticleMeta: &ArticleMetadata{LabelsHash: r.TaxonomyHash, Skipped: []ArticleRecord{{DocumentKey: "vn:vbpl:2", Anchor: "dieu-2", Outcome: emptyArticle("too_long")}}}}}, Results: map[string]Outcome{"request": {Articles: map[string]ArticleOutcome{"dieu-1": {Outcome: "ok", Codes: []CodeConfidence{{"1.1", 0.9}}, Dropped: []string{"unknown"}}}}}}
	dir := t.TempDir()
	if err := writeArticles(dir, s); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "articles.json")
	a, err := ReadArticleArtifact(path, r.TaxonomyHash, r.Article.FamiliesHash)
	if err != nil || len(a.Articles) != 2 || a.RecipeHash != r.Hash() || a.LabelsHash != r.TaxonomyHash {
		t.Fatalf("artifact %+v error %v", a, err)
	}
	if _, err = ReadArticleArtifact(path, r.TaxonomyHash, "wrong"); err == nil {
		t.Fatal("accepted different families")
	}
}

func TestFamilyArtifact_LegacyPromptRemainsRunnable(t *testing.T) {
	r := articleRecipe(t).documentRecipe()
	r.PromptVersion = 1
	dir := t.TempDir()
	if err := writeFamilies(dir, Submission{Recipe: r, Results: map[string]Outcome{"vn:vbpl:1": {Outcome: "ok", Codes: []string{"1.1"}}}}); err != nil {
		t.Fatal(err)
	}
	f, hash, err := ReadFamilyArtifact(filepath.Join(dir, "families.json"), r.TaxonomyHash, articleNodes())
	if err != nil {
		t.Fatal(err)
	}
	article, err := NewArticleRecipe(f.Recipe, hash)
	if err != nil {
		t.Fatal(err)
	}
	if err = article.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err = BuildArticleRequest(article, ArticleRequest{DocumentKey: "vn:vbpl:1", Articles: articleInputs(1, ""), Nodes: articleNodes()[1:4]}); err != nil {
		t.Fatal(err)
	}
}

func TestHistoricalArtifacts_AcceptedForScoring(t *testing.T) {
	r := articleRecipe(t)
	r.PromptVersion = 1
	r.Article.SourceRecipeHash = r.documentRecipe().Hash()
	dir := t.TempDir()
	if err := writeFamilies(dir, Submission{Recipe: r.documentRecipe(), Results: map[string]Outcome{"vn:vbpl:1": {Outcome: "ok", Codes: []string{"1.1"}}}}); err != nil {
		t.Fatal(err)
	}
	_, _, err := ReadFamilyArtifactForScore(filepath.Join(dir, "families.json"), r.TaxonomyHash, articleNodes())
	if err != nil {
		t.Fatal(err)
	}
	a := ArticleArtifact{Version: 1, Recipe: r, RecipeHash: r.Hash(), TaxonomyHash: r.TaxonomyHash, FamiliesHash: r.Article.FamiliesHash, Articles: []ArticleRecord{{DocumentKey: "vn:vbpl:1", Anchor: "dieu-1", Outcome: ArticleOutcome{Outcome: "ok", Codes: []CodeConfidence{{"1.1", 0.9}}}}}}
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	articleDir := t.TempDir()
	if err = writeOnce(filepath.Join(articleDir, "articles.json"), b); err != nil {
		t.Fatal(err)
	}
	if err = writeOnce(filepath.Join(articleDir, "MANIFEST.sha256"), []byte(digest(b)+"  articles.json\n")); err != nil {
		t.Fatal(err)
	}
	if _, err = ReadArticleArtifactForScore(filepath.Join(articleDir, "articles.json"), r.TaxonomyHash, r.Article.FamiliesHash); err != nil {
		t.Fatal(err)
	}
}
