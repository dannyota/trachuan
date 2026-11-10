package chunk

import (
	"encoding/json"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk/normalizer"
)

// Recipe identifies the deterministic chunking rules.
type Recipe struct {
	Version           string `json:"version"`
	RoughCount        string `json:"rough_count"`
	BodyLimit         int    `json:"body_limit"`
	MinimumRunes      int    `json:"minimum_runes"`
	Traversal         string `json:"traversal"`
	ParentLead        string `json:"parent_lead"`
	Sharding          string `json:"sharding"`
	Dedup             string `json:"dedup"`
	OutsideArticles   string `json:"outside_articles"`
	PrefixFields      string `json:"prefix_fields"`
	PrefixFieldLimit  int    `json:"prefix_field_limit"`
	PrefixNumberLimit int    `json:"prefix_number_limit"`
	DateNormalizer    string `json:"date_normalizer"`
}

// Recipe1 returns the baseline chunking recipe.
func Recipe1() Recipe {
	return Recipe{"1", "max(1,runes/2)", 512, 20, "parent-node-id-depth-first", "substantive-before-first-child", "lines-then-words", "nfc-collapsed-whitespace", "own-text", "number-title-chapter-section-date", 220, 128, normalizer.Version}
}

// Hash returns the recipe identity.
func (r Recipe) Hash() string {
	b, _ := json.Marshal(r)
	return stage.RecipeHash("chunk", "vn-article/1", string(b))
}
