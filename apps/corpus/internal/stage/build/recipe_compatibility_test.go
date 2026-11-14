package build

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/internal/retrieval"
)

func TestBuildRecipe_SupportsRetrievalRecipe2(t *testing.T) {
	s := New(Config{})
	snapshot := stage.Snapshot{SourceList: []byte(`{}`)}
	old := s.recipeForRetrieval(snapshot, retrieval.Recipe2())
	if old != "5854cd3e5f40f5ee3df4232e7fa8ba6bc97816dce9c1df4c7288bea1ae2f59ee" {
		t.Fatalf("recipe 2 build hash %s", old)
	}
	if !s.acceptsRecipeHash(snapshot, old) || !s.acceptsRecipeHash(snapshot, s.Recipe(snapshot)) || s.acceptsRecipeHash(snapshot, "other") {
		t.Fatal("build acceptance")
	}
}

func TestVerifyBuildArtifact_RejectsUnsupportedRetrievalRecipe(t *testing.T) {
	paired2 := retrieval.Recipe2()
	paired2.SiblingMerge = "article-clauses/1"
	paired3 := retrieval.Recipe3()
	paired3.SiblingMerge = ""
	changed := retrieval.Recipe3()
	changed.DocumentCap++
	for _, tc := range []struct {
		name   string
		recipe retrieval.Recipe
	}{{"recipe 2 with merge", paired2}, {"recipe 3 without merge", paired3}, {"changed value", changed}} {
		t.Run(tc.name, func(t *testing.T) {
			binding := "current-fixture"
			artifacts, art, run, cfg, _ := bindingArtifact(t, &binding, false, bindingOptions{recipe2: tc.recipe.Version == 2, recipe: &tc.recipe})
			if _, _, err := verifyBuildArtifact(artifacts, art, run, cfg); err == nil {
				t.Fatal("accepted unsupported retrieval recipe")
			}
		})
	}
}
