package build

import (
	"encoding/json"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/internal/retrieval"
)

func (s *Stage) acceptsRecipeHash(snapshot stage.Snapshot, hash string) bool {
	return hash == s.Recipe(snapshot) || hash == s.recipeForRetrieval(snapshot, retrieval.Recipe2())
}

func (s *Stage) recordRecipeHash(snapshot stage.Snapshot, raw json.RawMessage) string {
	var recipe retrieval.Recipe
	if json.Unmarshal(raw, &recipe) == nil && recipe.Version == 2 {
		return s.recipeForRetrieval(snapshot, retrieval.Recipe2())
	}
	return s.Recipe(snapshot)
}
