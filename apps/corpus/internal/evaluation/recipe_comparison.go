package evaluation

import (
	"encoding/json"
	"reflect"
)

func sameMRRRecipes(a, b *Report) bool {
	for _, key := range []string{"query", "document", "chunk", "lexical"} {
		if a.Recipes[key] == "" || a.Recipes[key] != b.Recipes[key] {
			return false
		}
	}
	before, after := b.RetrievalRecipe, a.RetrievalRecipe
	if len(before) == 0 && b.Version == 1 && a.Recipes["retrieval"] != "" && a.Recipes["retrieval"] == b.Recipes["retrieval"] {
		before = after
	}
	x, ok := comparisonRecipe(after)
	if !ok {
		return false
	}
	y, ok := comparisonRecipe(before)
	return ok && reflect.DeepEqual(x, y)
}

func comparisonRecipe(data json.RawMessage) (map[string]any, bool) {
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return nil, false
	}
	abstention, ok := fields["abstention"].(map[string]any)
	if !ok {
		return nil, false
	}
	delete(abstention, "calibration_sha256")
	return fields, true
}
