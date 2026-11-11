package embedding

import (
	"math"
	"testing"
)

func TestTestRecipe_HashAndIdentity(t *testing.T) {
	recipe := TestRecipe()
	if recipe.Model != "trachuan-test-only" || recipe.Runtime != "fake" {
		t.Fatalf("test recipe has model %q and runtime %q", recipe.Model, recipe.Runtime)
	}
	if TestRecipeHash == QueryRecipe1().Hash() || TestRecipeHash == DocumentRecipe1().Hash() {
		t.Fatal("test recipe collides with a pinned recipe")
	}
	if recipe.Hash() != TestRecipeHash {
		t.Fatalf("hash = %s, want %s", recipe.Hash(), TestRecipeHash)
	}
}

func TestFakeVector_DeterministicUnitLength(t *testing.T) {
	a, b, c := FakeVector("fixture"), FakeVector("fixture"), FakeVector("other")
	if len(a) != 1024 || len(b) != len(a) || len(c) != len(a) {
		t.Fatal("unexpected dimensions")
	}
	var norm float64
	different := false
	for i := range a {
		if a[i] != b[i] {
			t.Fatal("vector changed")
		}
		different = different || a[i] != c[i]
		norm += float64(a[i]) * float64(a[i])
	}
	if !different || math.Abs(norm-1) > 1e-6 {
		t.Fatalf("different=%t norm=%v", different, norm)
	}
}
