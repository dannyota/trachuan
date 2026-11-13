package build

import (
	"encoding/json"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/internal/retrieval"
)

func (s *Stage) acceptsRecipeHash(snapshot stage.Snapshot, hash string) bool {
	return hash == s.Recipe(snapshot) || hash == s.recipeForRetrieval(snapshot, retrieval.Recipe2())
}

func (s *Stage) recordRecipeHash(snapshot stage.Snapshot, raw json.RawMessage) (string, error) {
	var recipe retrieval.Recipe
	if err := json.Unmarshal(raw, &recipe); err != nil {
		return "", fmt.Errorf("decode retrieval recipe: %w", err)
	}
	if !recipe.Supported() {
		return "", fmt.Errorf("unsupported retrieval recipe")
	}
	if recipe.Version == 2 {
		return s.recipeForRetrieval(snapshot, retrieval.Recipe2()), nil
	}
	return s.Recipe(snapshot), nil
}
