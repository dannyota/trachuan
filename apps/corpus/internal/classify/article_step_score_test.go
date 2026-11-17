package classify

import (
	"path/filepath"
	"testing"
)

func TestArticleSteps_PartialFailureRemainsARecallMiss(t *testing.T) {
	labels, families, artifact, release, nodes := scoreFixture(t, 1)
	labels.Articles[0].Codes = []string{"1.1.c1.m1", "2.1"}
	families.Documents["vn:vbpl:1"] = Outcome{Outcome: "ok", Codes: []string{"1.1", "2.1"}}
	s := Submission{Country: "vn", Release: "20990101", Recipe: stepRecipe(t, 2), Settled: true, Items: []Item{
		{Key: "first", Article: &ArticleRequest{DocumentKey: "vn:vbpl:1", Articles: release}},
		{Key: "second", Article: &ArticleRequest{DocumentKey: "vn:vbpl:1", Articles: release}},
	}, Results: map[string]Outcome{"first": {Articles: map[string]ArticleOutcome{"dieu-1": artifact.Articles[0].Outcome}}, "second": {Outcome: "invalid_response"}}}
	dir := t.TempDir()
	if err := writeArticles(dir, s); err != nil {
		t.Fatal(err)
	}
	a, err := ReadArticleArtifactForScore(filepath.Join(dir, "articles.json"), s.Recipe.TaxonomyHash, s.Recipe.Article.FamiliesHash)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Score(labels, families, a, release, nodes)
	if err != nil {
		t.Fatal(err)
	}
	if report.Levels[2].Raw.TruePositive != 1 || report.Levels[2].Raw.FalseNegative != 1 || report.Levels[2].Raw.Recall != 0.5 {
		t.Fatalf("deepest score %+v", report.Levels[2])
	}
}
