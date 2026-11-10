package workflow

import (
	"encoding/json"
	"reflect"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func TestRegistry_ExpandAfterDiscover(t *testing.T) {
	got := Default(Deps{}).Names()
	want := []string{"discover", "expand", "fetch", "extract", "relate", "chunk", "embed", "build", "eval"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage order = %v", got)
	}
}

func TestRecipes_SourceListJSONBFormattingDoesNotChangeDiscover(t *testing.T) {
	before := stage.Snapshot{SourceList: json.RawMessage(`{"vbpl":{"selection_version":1},"chinhphu":{"named_numbers":["142/2026/NĐ-CP"]}}`), SourceListHash: "source-hash"}
	after := before
	after.SourceList = json.RawMessage(`{"vbpl": {"selection_version": 1}, "chinhphu": {"named_numbers": ["142/2026/NĐ-CP"]}}`)
	registry := Default(Deps{})
	start, worker := registry.Recipes(before)["discover"], registry.Recipes(after)["discover"]
	if start != worker {
		t.Fatalf("discover recipe changed after jsonb formatting: %s != %s", start, worker)
	}
}
