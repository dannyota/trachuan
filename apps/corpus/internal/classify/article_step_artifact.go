package classify

import (
	"encoding/json"
	"fmt"
)

func articleInputHash(country, release, labelsHash string, articles []Article) string {
	type selected struct{ DocumentKey, Anchor, Heading, Text, Chapter, ChapterHeading string }
	selection := make([]selected, 0, len(articles))
	for _, a := range articles {
		selection = append(selection, selected{a.DocumentKey, a.Anchor, a.Heading, a.Text, a.Chapter, a.ChapterHeading})
	}
	b, _ := json.Marshal(struct {
		Country, Release, LabelsHash string
		Articles                     []selected
	}{country, release, labelsHash, selection})
	return digest(b)
}

func ReadStep1Artifact(path, taxonomyHash, familiesHash string) (ArticleArtifact, string, error) {
	var a ArticleArtifact
	hash, err := readArtifact(path, &a)
	if err != nil {
		return a, "", fmt.Errorf("read step-1 artifact: %w", err)
	}
	if a.Recipe.Article == nil || a.Recipe.Article.Step != 1 || a.Recipe.Article.PromptVersion != 3 || !a.Settled || a.InputHash == "" {
		return a, "", fmt.Errorf("step-1 artifact requires settled prompt-v3 family choices and input evidence")
	}
	if err = validateArticleArtifact(a, taxonomyHash, familiesHash, false); err != nil {
		return a, "", fmt.Errorf("validate step-1 artifact: %w", err)
	}
	return a, hash, nil
}

func readArticleChoices(o Options, labelsHash string, articles []Article, families FamilyArtifact, nodes []Node) (map[string]Outcome, error) {
	if o.Step1Artifact == "" {
		return nil, fmt.Errorf("step 2 requires a verified step-1 artifact")
	}
	a, hash, err := ReadStep1Artifact(o.Step1Artifact, o.Recipe.TaxonomyHash, o.Recipe.Article.FamiliesHash)
	if err != nil {
		return nil, err
	}
	if hash != o.Recipe.Article.Step1Hash || a.Country != o.Country || a.Release != o.Release || a.LabelsHash != labelsHash || a.InputHash != articleInputHash(o.Country, o.Release, labelsHash, articles) || a.Recipe.Article.SourceRecipeHash != o.Recipe.Article.SourceRecipeHash {
		return nil, fmt.Errorf("step-1 artifact differs from recipe, release or selected inputs")
	}
	selected := map[string]bool{}
	for _, article := range articles {
		selected[article.DocumentKey+"/"+article.Anchor] = true
	}
	if len(a.Articles) != len(selected) {
		return nil, fmt.Errorf("step-1 artifact does not cover selected articles")
	}
	choices := map[string]Outcome{}
	for _, record := range a.Articles {
		if record.Outcome.FailedFamilies != nil {
			return nil, fmt.Errorf("step-1 artifact cannot carry failed node families")
		}
		key := record.DocumentKey + "/" + record.Anchor
		if !selected[key] {
			return nil, fmt.Errorf("step-1 artifact names an unselected article")
		}
		menu, menuErr := allowedNodes(families.Documents[record.DocumentKey].Codes, nodes)
		if menuErr != nil {
			return nil, menuErr
		}
		allowed := map[string]bool{}
		for _, n := range menu {
			if n.Level == 2 {
				allowed[n.Code] = true
			}
		}
		choice := Outcome{Outcome: record.Outcome.Outcome}
		for _, code := range record.Outcome.Codes {
			if !allowed[code.Code] {
				return nil, fmt.Errorf("step-1 artifact names a node outside its document families")
			}
			choice.Codes = append(choice.Codes, code.Code)
		}
		choices[key] = choice
	}
	return choices, nil
}
