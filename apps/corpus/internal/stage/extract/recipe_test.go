package extract

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func TestRecipe_TextParserVersion(t *testing.T) {
	got := New().Recipe(stage.Snapshot{})
	want := stage.RecipeHash("extract", "tree/1", "html-transcription/1", "textparse/2", "ocr-store/1", "", "vision-ocr/builtin-latest", "vision-ocr/builtin-latest/poppler-200")
	old := stage.RecipeHash("extract", "tree/1", "html-transcription/1", "textparse/1", "ocr-absent/1")
	if got != want || got == old {
		t.Fatalf("recipe %s want %s", got, want)
	}
}
