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
	if got != "f5d9df5d774886777b318feb7b5311377cf42b2b79f15bf5f1f2ca294fbdab1f" {
		t.Fatalf("build recipe = %s", got)
	}
}
