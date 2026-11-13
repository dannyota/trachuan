package retrieval

import "fmt"

// ArticleCapOverride sets a research-only article cap. Zero means no cap.
type ArticleCapOverride CapOverride

// ParseArticleCapOverride parses a cap from 1 to 50 or none.
func ParseArticleCapOverride(s string) (*ArticleCapOverride, error) {
	c, err := ParseCapOverride(s)
	if err != nil {
		return nil, fmt.Errorf("invalid article cap: %w", err)
	}
	v := ArticleCapOverride(*c)
	return &v, nil
}

// MarshalJSON encodes the cap as an integer or none.
func (c ArticleCapOverride) MarshalJSON() ([]byte, error) { return CapOverride(c).MarshalJSON() }

// UnmarshalJSON decodes an integer cap or none.
func (c *ArticleCapOverride) UnmarshalJSON(data []byte) error {
	var v CapOverride
	if err := v.UnmarshalJSON(data); err != nil {
		return fmt.Errorf("invalid article cap: %w", err)
	}
	*c = ArticleCapOverride(v)
	return nil
}

// RankWithArticleCap ranks cached candidates with research-only caps.
func RankWithArticleCap(arms Arms, recipe Recipe, limit int, mode ArmMode, document *CapOverride, article *ArticleCapOverride) (Ranking, error) {
	if limit == 0 {
		limit = 8
	}
	cap, err := effectiveCap(document, recipe.DocumentCap, limit)
	if err != nil {
		return Ranking{}, err
	}
	if err := validateArticleCap(article); err != nil {
		return Ranking{}, err
	}
	return rankWithCap(arms, recipe, limit, mode, cap, article)
}

func validateArticleCap(cap *ArticleCapOverride) error {
	if cap != nil && (*cap < 0 || *cap > 50) {
		return fmt.Errorf("invalid article cap override")
	}
	return nil
}

// ShapeWithArticleCap shapes cached candidates using research-only policies.
// A nil tail preserves the recipe policy.
func ShapeWithArticleCap(arms Arms, recipe Recipe, limit int, mode ArmMode, document *CapOverride, tail *int, article *ArticleCapOverride) (Response, error) {
	if limit == 0 {
		limit = 8
	}
	cap, err := effectiveCap(document, recipe.DocumentCap, limit)
	if err != nil {
		return Response{}, err
	}
	if err = validateArticleCap(article); err != nil {
		return Response{}, err
	}
	value := recipe.NonCurrentTail
	if tail != nil {
		value = *tail
		arms.IncludeNonCurrent = false
	}
	return shapeWithPolicy(arms, recipe, limit, mode, cap, value, article)
}
