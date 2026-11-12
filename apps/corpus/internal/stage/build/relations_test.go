package build

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/relate"
)

func TestRelationReaders_ValidatePair(t *testing.T) {
	ctx := context.Background()
	store := artifact.Store{Root: t.TempDir()}
	publish := func(stageName string, files map[string]string) artifact.Result {
		t.Helper()
		a, err := store.Publish(ctx, stageName, strings.Repeat("a", 64), strings.Repeat("b", 64), func(dir string) error {
			for name, data := range files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	f := publish("fetch", map[string]string{"detail.json": `{"data":{"id":"1"}}`, "files.json": `[]`})
	r := publish("relate", map[string]string{"relations.json": `[]`})
	key := "vn:vbpl:1"
	fetch := InventoryItem{Stage: "fetch", ItemKey: key, State: "done", Path: &f.Path, ArtifactSHA256: &f.SHA256}
	extract := InventoryItem{Stage: "extract", ItemKey: key, State: "gap"}
	extractInput := stage.RecipeHash("extract-input", "1", f.SHA256, "none", "absent", "no_text_source")
	e := publish("extract", map[string]string{"text.json": `{"text_source":"none","gap":"no_text_source"}`, "provisions.json": `[]`})
	done := InventoryItem{Stage: "extract", ItemKey: key, State: "done", Path: &e.Path, ArtifactSHA256: &e.SHA256, InputHash: &extractInput}
	pair := relate.PairInputHash(f.SHA256, "-")
	donePair := relate.PairInputHash(f.SHA256, e.SHA256)
	relation := InventoryItem{Stage: "relate", ItemKey: key, State: "done", Path: &r.Path, ArtifactSHA256: &r.SHA256, InputHash: &pair}
	for _, tc := range []struct {
		name                  string
		hash                  *string
		extract               InventoryItem
		includeExtract, valid bool
	}{
		{"gap pair", &pair, extract, true, true}, {"done pair", &donePair, done, true, true},
		{"changed extract", &pair, done, true, false}, {"fetch only", &f.SHA256, extract, true, false},
		{"missing extract", &donePair, extract, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			relation.InputHash = tc.hash
			items := []InventoryItem{fetch, relation}
			byStage := map[string]map[string]InventoryItem{"fetch": {key: fetch}, "relate": {key: relation}}
			if tc.includeExtract {
				items = append(items, tc.extract)
				byStage["extract"] = map[string]InventoryItem{key: tc.extract}
			}
			l := loader{artifacts: store, items: byStage}
			if err := l.loadRelations(ctx); (err == nil) != tc.valid {
				t.Errorf("loadRelations error = %v", err)
			}
			inv := Inventory{ExpectedKeys: []string{key}, Items: items}
			if _, err := IdentityInventory(inv, store); (err == nil) != tc.valid {
				t.Errorf("IdentityInventory error = %v", err)
			}
		})
	}
}

func TestResolveNumber_RequiresUniqueMatch(t *testing.T) {
	numbers := map[string][]string{"01/2024/TT": {"vn:vbpl:1", "vn:vbpl:2"}, "02/2024/TT": {"vn:vbpl:3"}}
	if got := resolveNumber(numbers, "01/2024/TT"); got != "" {
		t.Fatalf("ambiguous number resolved to %q", got)
	}
	if got := resolveNumber(numbers, "02/2024/TT"); got != "vn:vbpl:3" {
		t.Fatalf("unique number resolved to %q", got)
	}
}

func TestRelations_NumberIndexUsesLoadedDocuments(t *testing.T) {
	l := loader{docs: map[string]*loadedDocument{"vn:vbpl:1": {number: "01/2024/TT"}, "vn:vbpl:2": {number: "02/2024/TT"}}}
	numbers := l.documentNumbers()
	if got := resolveNumber(numbers, "01/2024/TT"); got != "vn:vbpl:1" {
		t.Fatalf("number resolved to %q", got)
	}
}
