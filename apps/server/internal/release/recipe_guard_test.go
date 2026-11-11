package release

import (
	"testing"

	"danny.vn/trachuan/internal/embedding"
)

func TestRecipeAllowed_LoopbackOnly(t *testing.T) {
	for _, tc := range []struct {
		name, hash     string
		loopback, want bool
	}{
		{"test on public deployment", embedding.TestRecipeHash, false, false},
		{"test on loopback", embedding.TestRecipeHash, true, true},
		{"real on public deployment", embedding.QueryRecipe1().Hash(), false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := recipeAllowed(tc.hash, tc.loopback); got != tc.want {
				t.Fatalf("allowed = %t, want %t", got, tc.want)
			}
		})
	}
}
