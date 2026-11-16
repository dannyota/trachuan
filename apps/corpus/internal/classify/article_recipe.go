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
}

func NewArticleRecipe(base Recipe, familiesHash string) (Recipe, error) {
	if base.Article != nil {
		return Recipe{}, fmt.Errorf("article recipe requires a document recipe")
	}
	if err := base.Validate(); err != nil {
		return Recipe{}, err
	}
	base.Article = &ArticleRecipe{SourceRecipeHash: base.Hash(), FamiliesHash: familiesHash, PromptVersion: 1, SchemaVersion: 1, MaxTokens: 8192}
	return base, base.Validate()
}

func (r Recipe) documentRecipe() Recipe { r.Article = nil; return r }

func (r Recipe) validateArticle() error {
	a := r.Article
	h, err := hex.DecodeString(a.FamiliesHash)
	if err != nil || len(h) != 32 || a.SourceRecipeHash != r.documentRecipe().Hash() || a.PromptVersion != 1 || a.SchemaVersion != 1 || a.MaxTokens < 1 || a.MaxTokens > 32000 {
		return fmt.Errorf("invalid article classification recipe")
	}
	return nil
}

func (r Recipe) outputTokens() int64 {
	if r.Article != nil {
		return r.Article.MaxTokens
	}
	return r.MaxTokens
}
