package extract

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func TestRecipe_ReleaseChecker(t *testing.T) {
	const releaseChecker = "claude-opus-5-5/medium/ocr-check-2"
	if ReleaseChecker != releaseChecker {
		t.Fatalf("release checker %q want %q", ReleaseChecker, releaseChecker)
	}
	want := stage.RecipeHash("extract", "tree/1", "html-transcription/1", "textparse/2", "ocr-store/1", releaseChecker, "vision-ocr/builtin-latest", "vision-ocr/builtin-latest/poppler-200", "vision-ocr/builtin-latest/poppler-200-jpeg")
	store := ocrstore.Open(t.TempDir(), nil)
	for _, tc := range []struct {
		name  string
		stage *Stage
	}{
		{"without store", New()},
		{"with store and empty checker", New().WithOCR(store, nil, "")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.stage.Recipe(stage.Snapshot{}); got != want {
				t.Fatalf("recipe %s want %s", got, want)
			}
		})
	}
	s := New().WithOCR(store, nil, "other")
	overridden := s.Recipe(stage.Snapshot{})
	if overridden == want || s.checker != "other" {
		t.Fatal("non-empty checker did not override the release checker")
	}
	if s.WithOCR(store, nil, "").Recipe(stage.Snapshot{}) != overridden {
		t.Fatal("empty checker cleared the current override")
	}
}

func TestRecipe_TextParserVersion(t *testing.T) {
	got := New().Recipe(stage.Snapshot{})
	want := stage.RecipeHash("extract", "tree/1", "html-transcription/1", "textparse/2", "ocr-store/1", ReleaseChecker, "vision-ocr/builtin-latest", "vision-ocr/builtin-latest/poppler-200", "vision-ocr/builtin-latest/poppler-200-jpeg")
	old := stage.RecipeHash("extract", "tree/1", "html-transcription/1", "textparse/1", "ocr-absent/1")
	if got != want || got == old {
		t.Fatalf("recipe %s want %s", got, want)
	}
}
