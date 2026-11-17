package classify

import (
	"strings"
	"testing"
)

func TestRecipe_BindsEveryRequestSetting(t *testing.T) {
	r, err := NewRecipe("claude-haiku-5-5", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	base := r.Hash()
	variants := []Recipe{r, r, r, r, r, r, r, r}
	variants[0].Model = "claude-sonnet-5-5"
	variants[1].Effort = "low"
	variants[2].MaxTokens++
	variants[3].InputBound++
	variants[4].F++
	variants[5].PromptVersion++
	variants[6].SchemaVersion++
	variants[7].TaxonomyHash = strings.Repeat("b", 64)
	for i, v := range variants {
		if v.Hash() == base {
			t.Fatalf("setting %d is unbound", i)
		}
	}
	if _, err = NewRecipe("unsupported", r.TaxonomyHash); err == nil {
		t.Fatal("accepted unsupported model")
	}
}

func TestReserve_RoundsUpAndRejectsTierCrossing(t *testing.T) {
	r, _ := NewRecipe("claude-haiku-5-5", strings.Repeat("a", 64))
	r.MaxTokens = 1
	for _, tc := range []struct{ input, want int64 }{{1, 1}, {20, 2}, {99999, 5001}} {
		r.InputBound = 99999
		got, err := r.Reserve(tc.input)
		if err != nil || got != tc.want {
			t.Fatalf("input %d: cost %d error %v", tc.input, got, err)
		}
	}
	if _, err := r.Reserve(100000); err == nil {
		t.Fatal("accepted upper tier input")
	}
}

func TestRecipe_DocumentPromptVersion(t *testing.T) {
	r, err := NewRecipe("claude-haiku-5-5", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	old := r
	old.PromptVersion = 1
	if r.PromptVersion != 2 || r.Hash() == old.Hash() || old.Validate() == nil {
		t.Fatalf("current %+v old %+v", r, old)
	}
}
