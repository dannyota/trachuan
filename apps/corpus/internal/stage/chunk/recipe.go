package chunk

import (
	"encoding/json"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk/normalizer"
)

type Recipe struct {
	Version             int      `json:"version"`
	RoughCount          string   `json:"rough_count"`
	BodyLimit           int      `json:"body_limit"`
	MinimumRunes        int      `json:"minimum_runes"`
	Traversal           string   `json:"traversal"`
	ParentLead          string   `json:"parent_lead"`
	Sharding            string   `json:"sharding"`
	Dedup               string   `json:"dedup"`
	OutsideArticles     string   `json:"outside_articles"`
	PrefixFields        string   `json:"prefix_fields"`
	PrefixFieldLimit    int      `json:"prefix_field_limit"`
	PrefixNumberLimit   int      `json:"prefix_number_limit"`
	DateNormalizer      string   `json:"date_normalizer"`
	ListingOnlyDocTypes []string `json:"listing_only_doc_types"`
}

func Recipe2() Recipe {
	return Recipe{2, "max(1,runes/2)", 512, 20, "parent-node-id-depth-first", "substantive-before-first-child", "lines-then-words", "nfc-collapsed-whitespace", "own-text", "number-title-chapter-section-date", 220, 128, normalizer.Version, []string{"Văn bản hệ thống hóa"}}
}

func (r Recipe) Hash() string {
	b, _ := json.Marshal(r)
	return stage.RecipeHash("chunk", "vn-article/2", string(b))
}
