package build

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func TestBuildRecipe_ProjectionVersion(t *testing.T) {
	got := New(Config{}).Recipe(stage.Snapshot{SourceList: []byte(`{}`)})
	if got == "32be8523e5288ab734afb45ed7d6ce3b0c45cdc51d78331438425bf11a10cb1a" {
		t.Fatal("projection did not change build recipe")
	}
	if got != "6deaedd1c14022a22c941c53722ffc5eeaf0e5e423cf9eecd429e0f23b5d5f24" {
		t.Fatalf("build recipe = %s", got)
	}
}
