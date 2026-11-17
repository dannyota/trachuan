package classify

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

type Recipe struct {
	Version       int            `json:"version"`
	Model         string         `json:"model"`
	Effort        string         `json:"effort"`
	MaxTokens     int64          `json:"max_tokens"`
	InputBound    int64          `json:"input_bound"`
	F             int            `json:"families"`
	PromptVersion int            `json:"prompt_version"`
	SchemaVersion int            `json:"schema_version"`
	TaxonomyHash  string         `json:"taxonomy_hash"`
	Article       *ArticleRecipe `json:"article,omitempty"`
}

func NewRecipe(model, taxonomyHash string) (Recipe, error) {
	r := Recipe{Version: 1, Model: model, Effort: "medium", MaxTokens: 4096, InputBound: 90000, F: 5, PromptVersion: 2, SchemaVersion: 2, TaxonomyHash: taxonomyHash}
	return r, r.Validate()
}

func (r Recipe) Validate() error {
	return r.validate(false)
}

func (r Recipe) validate(historical bool) error {
	if r.Model != "claude-haiku-5-5" && r.Model != "claude-sonnet-5-5" {
		return fmt.Errorf("unsupported classification model")
	}
	if r.Effort != "low" && r.Effort != "medium" && r.Effort != "high" && r.Effort != "xhigh" && r.Effort != "max" {
		return fmt.Errorf("invalid classification effort")
	}
	if r.Version != 1 || (r.PromptVersion != 2 && ((!historical && r.Article == nil) || r.PromptVersion != 1)) || r.SchemaVersion != 2 || r.MaxTokens < 1 || r.MaxTokens > 32000 || r.InputBound < 1 || r.InputBound >= 100000 || r.F < 1 {
		return fmt.Errorf("invalid classification recipe")
	}
	h, err := hex.DecodeString(r.TaxonomyHash)
	if err != nil || len(h) != 32 {
		return fmt.Errorf("invalid taxonomy hash")
	}
	if r.Article != nil {
		if err := r.validateArticle(); err != nil {
			return err
		}
	}
	return nil
}

func (r Recipe) Hash() string {
	b, _ := json.Marshal(r)
	return digest(b)
}

func digest(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (r Recipe) Reserve(input int64) (int64, error) {
	if err := r.Validate(); err != nil {
		return 0, err
	}
	if input < 1 || input > r.InputBound {
		return 0, fmt.Errorf("counted input exceeds classification recipe bound or is empty")
	}
	return r.cost(input, r.outputTokens()), nil
}

func (r Recipe) cost(input, output int64) int64 {
	// Anthropic Batch prices: https://platform.claude.com/docs/en/about-claude/pricing
	if r.Model == "claude-haiku-5-5" {
		return (input + 5*output + 19) / 20
	}
	return input + 5*output
}
