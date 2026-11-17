package classify

import (
	"encoding/hex"
	"fmt"
)

type ArticleRecipe struct {
	SourceRecipeHash string `json:"source_recipe_hash"`
	FamiliesHash     string `json:"families_sha256"`
	PromptVersion    int    `json:"prompt_version"`
	SchemaVersion    int    `json:"schema_version"`
	MaxTokens        int64  `json:"max_tokens"`
	Step             int    `json:"step,omitempty"`
	Step1Hash        string `json:"step_1_sha256,omitempty"`
}

func NewArticleRecipe(base Recipe, familiesHash string) (Recipe, error) {
	if base.Article != nil {
		return Recipe{}, fmt.Errorf("article recipe requires a document recipe")
	}
	if err := base.validate(true); err != nil {
		return Recipe{}, err
	}
	base.Article = &ArticleRecipe{SourceRecipeHash: base.Hash(), FamiliesHash: familiesHash, PromptVersion: 1, SchemaVersion: 1, MaxTokens: 8192}
	return base, base.Validate()
}

func (r Recipe) documentRecipe() Recipe { r.Article = nil; return r }

func (r Recipe) validateArticle() error {
	a := r.Article
	h, err := hex.DecodeString(a.FamiliesHash)
	if err != nil || len(h) != 32 || a.SourceRecipeHash != r.documentRecipe().Hash() || a.SchemaVersion != 1 || a.MaxTokens < 1 || a.MaxTokens > 32000 {
		return fmt.Errorf("invalid article classification recipe")
	}
	if a.PromptVersion == 1 && a.Step == 0 && a.Step1Hash == "" {
		return nil
	}
	if a.PromptVersion != 3 || r.Model != "claude-haiku-5-5" || (a.Step != 1 && a.Step != 2) {
		return fmt.Errorf("invalid two-step article recipe")
	}
	if a.Step == 1 && a.Step1Hash == "" {
		return nil
	}
	h, err = hex.DecodeString(a.Step1Hash)
	if a.Step != 2 || err != nil || len(h) != 32 {
		return fmt.Errorf("step 2 requires the step-1 artifact hash")
	}
	return nil
}

func NewTwoStepArticleRecipe(base Recipe, familiesHash string, step int, step1Hash string) (Recipe, error) {
	r, err := NewArticleRecipe(base, familiesHash)
	if err != nil {
		return Recipe{}, err
	}
	r.Article.PromptVersion = 3
	r.Article.Step = step
	r.Article.Step1Hash = step1Hash
	return r, r.Validate()
}

func (r Recipe) outputTokens() int64 {
	if r.Article != nil {
		return r.Article.MaxTokens
	}
	return r.MaxTokens
}
