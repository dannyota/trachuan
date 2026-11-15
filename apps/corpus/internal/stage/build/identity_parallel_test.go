package build

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/apps/corpus/internal/stage/relate"
)

func TestIdentityInventory_ParallelMatchesSerialAndFirstError(t *testing.T) {
	store := artifact.Store{Root: t.TempDir()}
	inv := Inventory{}
	for i := 1; i <= 3; i++ {
		key := fmt.Sprintf("vn:vbpl:%d", i)
		input := strings.Repeat(fmt.Sprint(i), 64)
		art, err := store.Publish(context.Background(), "fetch", input, strings.Repeat("a", 64), func(dir string) error {
			if err := os.WriteFile(filepath.Join(dir, "detail.json"), []byte(fmt.Sprintf(`{"data":{"id":"%d"}}`, i)), 0600); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(dir, "files.json"), []byte(`[]`), 0600)
		})
		if err != nil {
			t.Fatal(err)
		}
		inv.ExpectedKeys = append(inv.ExpectedKeys, key)
		inv.Items = append(inv.Items, InventoryItem{Stage: "fetch", State: "done", ItemKey: key, Path: &art.Path, ArtifactSHA256: &art.SHA256})
		extractInput := art.SHA256
		ext, err := store.Publish(context.Background(), "extract", extractInput, strings.Repeat("a", 64), func(dir string) error {
			return os.WriteFile(filepath.Join(dir, "provisions.json"), []byte(fmt.Sprintf(`[{"node_id":"node%d","path":"path%d","text":"text%d"},{"node_id":"other","path":"common","text":"other text"}]`, i, i, i)), 0600)
		})
		if err != nil {
			t.Fatal(err)
		}
		inv.Items = append(inv.Items, InventoryItem{Stage: "extract", State: "done", ItemKey: key, Path: &ext.Path, ArtifactSHA256: &ext.SHA256, InputHash: &extractInput})
		relationInput := relate.PairInputHash(art.SHA256, ext.SHA256)
		rel, err := store.Publish(context.Background(), "relate", relationInput, strings.Repeat("a", 64), func(dir string) error {
			return os.WriteFile(filepath.Join(dir, "relations.json"), []byte(`[]`), 0600)
		})
		if err != nil {
			t.Fatal(err)
		}
		inv.Items = append(inv.Items, InventoryItem{Stage: "relate", State: "done", ItemKey: key, Path: &rel.Path, ArtifactSHA256: &rel.SHA256, InputHash: &relationInput})
	}
	serial, err := identityInventory(inv, store, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	for key, doc := range serial.Documents {
		if len(doc.Texts) != 0 {
			t.Fatalf("unreferenced text retained for %s", key)
		}
	}
	anchor := "common"
	mapping := evaluation.IdentityMap{Entries: []evaluation.MapEntry{{Evidence: []evaluation.Evidence{{DocumentKey: "vn:vbpl:1", Anchor: &anchor}, {DocumentKey: "vn:vbpl:2"}}}}}
	anchors := mapping.EvidenceAnchors()
	serial, err = identityInventory(inv, store, anchors, 1)
	if err != nil {
		t.Fatal(err)
	}
	parallel, err := identityInventory(inv, store, anchors, 8)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(serial, parallel) {
		t.Fatal("parallel inventory differs")
	}
	for i, key := range inv.ExpectedKeys {
		doc := parallel.Documents[key]
		path := fmt.Sprintf("path%d", i+1)
		if !doc.Anchors[path] || !doc.Anchors["common"] || (i == 0 && (doc.Texts["common"] != "other text" || len(doc.Texts) != 1)) || (i != 0 && len(doc.Texts) != 0) {
			t.Fatalf("missing extracted identity for %s", key)
		}
	}
	originalSecond, originalThird := inv.Items[3].Path, inv.Items[6].Path
	inv.Items[3].Path, inv.Items[6].Path = nil, nil
	_, first := identityInventory(inv, store, nil, 1)
	_, second := identityInventory(inv, store, nil, 8)
	if first == nil || second == nil || first.Error() != second.Error() || !strings.Contains(second.Error(), "vn:vbpl:2") {
		t.Fatalf("first=%v parallel=%v", first, second)
	}
	inv.Items[3].Path, inv.Items[6].Path = originalSecond, originalThird
	inv.Items[4].Path, inv.Items[7].Path = nil, nil
	_, first = identityInventory(inv, store, nil, 1)
	_, second = identityInventory(inv, store, nil, 8)
	if first == nil || second == nil || first.Error() != second.Error() || !strings.Contains(second.Error(), "vn:vbpl:2") {
		t.Fatalf("extract first=%v parallel=%v", first, second)
	}
}
