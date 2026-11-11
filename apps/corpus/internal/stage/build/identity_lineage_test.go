package build

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func TestIdentityInventory_ExtractSelectionLineage(t *testing.T) {
	ctx := context.Background()
	store := artifact.Store{Root: t.TempDir()}
	recipe := strings.Repeat("a", 64)
	publish := func(stageName, input string, files map[string]string) artifact.Result {
		t.Helper()
		art, err := store.Publish(ctx, stageName, input, recipe, func(dir string) error {
			for name, body := range files {
				if e := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); e != nil {
					return e
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return art
	}
	f := publish("fetch", strings.Repeat("b", 64), map[string]string{"detail.json": `{"data":{"id":"1"}}`, "files.json": `[]`})
	input := stage.RecipeHash("extract-input", "1", f.SHA256, "none", "absent", "no_text_source")
	e := publish("extract", input, map[string]string{"text.json": `{"text_source":"none","gap":"no_text_source"}`, "provisions.json": `[]`})
	inv := Inventory{ExpectedKeys: []string{"vn:vbpl:1"}, Items: []InventoryItem{{Stage: "fetch", ItemKey: "vn:vbpl:1", State: "done", Path: &f.Path, ArtifactSHA256: &f.SHA256}, {Stage: "extract", ItemKey: "vn:vbpl:1", State: "done", Path: &e.Path, ArtifactSHA256: &e.SHA256, InputHash: &input}}}
	if _, err := IdentityInventory(inv, store); err != nil {
		t.Fatal(err)
	}
}
