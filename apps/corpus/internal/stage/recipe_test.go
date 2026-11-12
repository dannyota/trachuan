package stage

import (
	"errors"
	"testing"
)

func TestRecipe_RefusesChangedOrMissingFrozenRecipe(t *testing.T) {
	for _, tc := range []struct{ name, frozen, compiled, want string }{
		{"changed", "old", "new", "recipe mismatch for stage extract"},
		{"missing", "", "new", "missing frozen recipe for stage extract"},
		{"matching", "same", "same", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := Snapshot{Recipes: map[string]string{}}
			if tc.frozen != "" {
				snapshot.Recipes["extract"] = tc.frozen
			}
			err := checkRecipe(snapshot, "extract", tc.compiled)
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var refusal *RecipeError
			if err == nil || err.Error() != tc.want || !errors.As(err, &refusal) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
