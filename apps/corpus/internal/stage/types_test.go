package stage

import "testing"

func TestRecipeHash_Stable(t *testing.T) {
	// SHA-256 of the bytes "fetch\x00v1\x00extra".
	const want = "c401cb8b0db5ef8c50f444720aaae299c9f288d0c7b411b97b920c268119f0b2"
	if got := RecipeHash("fetch", "v1", "extra"); got != want {
		t.Errorf("RecipeHash() = %q, want %q", got, want)
	}
}
