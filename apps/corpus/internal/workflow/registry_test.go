package workflow

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/evidence"
)

func TestRegistry_ExpandAfterDiscover(t *testing.T) {
	got := Default(Deps{}).Names()
	want := []string{"discover", "expand", "fetch", "extract", "relate", "chunk", "embed", "build", "eval", "evidence"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage order = %v", got)
	}
}

func TestRecipes_ExistingStagesUnchangedByEvidence(t *testing.T) {
	want := map[string]string{
		"discover": "09da6b51d9c2d76834aeaa9287713f5ec14637d1461c679a55118be7210caaf1",
		"expand":   "dcf048253bfeb4978175a00f122adaeeb086bd9d7b65cea20d88ee13951627bd",
		"fetch":    "1f6699b0264c7ff2550542ccf2689868c1d40e435464151edcfa3dd991b9b351",
		"extract":  "88fbe0aa91d42f5161ea30f946522a9a9defcda24b6b444f89a0dec01b596abf",
		"relate":   "6c431204e691cdea5cea35c940583cd8d8663358bf0cd07e92a779da51773fe2",
		"chunk":    "40c50d47068734972075a0a0a6cd78b1e5dc55fc43bfb4dfce34944049336d82",
		"embed":    "11496930ffabf11a1ffd3b3f31cf155f616d15ef6272a5ff35373ba917a2338d",
		"build":    "68fa166c5587645be0e8474d9df08cf22d5a10eff13c173150508988360c92fb",
		"eval":     "58f7f5dac2ab0f829646c48babe81d19d8e173389e1bbf50e273bc57892ecd32",
	}
	snapshot := stage.Snapshot{SourceList: json.RawMessage(`{"country":"vn","vbpl":{"selection_version":1},"status_codes":{}}`), SourceListHash: "source-hash"}
	decided := snapshot
	decided.PreviousReport = &stage.PreviousReport{SHA256: strings.Repeat("a", 64), Path: "previous-report/x"}
	for _, s := range []stage.Snapshot{snapshot, decided} {
		got := Default(Deps{}).Recipes(s)
		for name, recipe := range want {
			if got[name] != recipe {
				t.Errorf("%s recipe = %s, want %s", name, got[name], recipe)
			}
		}
		if got[stage.EvidenceStage] == "" {
			t.Error("evidence recipe missing")
		}
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

func TestRegistry_EvidenceStageRunsCrossSignalDetectors(t *testing.T) {
	var snapshot stage.Snapshot
	got := Default(Deps{}).Recipes(snapshot)["evidence"]
	if want := evidence.New(evidence.Config{Detectors: evidence.CrossSignal()}).Recipe(snapshot); got != want {
		t.Fatalf("evidence recipe %s does not freeze the cross-signal detectors (%s)", got, want)
	}
	if empty := evidence.New(evidence.Config{}).Recipe(snapshot); got == empty {
		t.Fatal("evidence recipe ignores the detector list")
	}
}
