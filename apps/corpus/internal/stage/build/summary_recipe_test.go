package build

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildRecipe_RejectsPreBoundsArtifact(t *testing.T) {
	for _, recipe2 := range []bool{false, true} {
		t.Run(map[bool]string{false: "retrieval_3", true: "retrieval_2"}[recipe2], func(t *testing.T) {
			binding := "current-fixture"
			old := "dd47d6f8b8b73470f274cfaf438158986e3760a6187107a117cae30feb023665"
			if recipe2 {
				old = "ec98f52f69745364842a6734091b6e8714eb0930b2eb8e8a09cd1a31b243a1ed"
			}
			artifacts, art, run, cfg, _ := bindingArtifact(t, &binding, false, bindingOptions{recipe2: recipe2, buildRecipeHash: old})
			if New(cfg).acceptsRecipeHash(run.Snapshot, old) {
				t.Error("accepted pre-bounds recipe")
			}
			if _, _, err := verifyBuildArtifact(artifacts, art, run, cfg); err == nil || !strings.Contains(err.Error(), "build artifact identity mismatch") {
				t.Errorf("pre-bounds artifact error=%v", err)
			}
			raw, err := json.Marshal(map[string]any{"items": []any{map[string]any{"stage": "build", "item_key": "vn/20990101", "state": "done", "path": art.Path, "input_hash": art.InputHash, "recipe_hash": old, "artifact_sha256": art.SHA256}}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = VerifyForRun(context.Background(), cfg, inventoryQueryStub{raw: raw}, artifacts, run); err == nil {
				t.Error("adopted committed pre-bounds artifact")
			}
		})
	}
}
