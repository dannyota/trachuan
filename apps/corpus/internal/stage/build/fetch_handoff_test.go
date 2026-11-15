package build

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage/relate"
)

func TestFetchHandoff_RelationsUseVerifiedDocuments(t *testing.T) {
	ctx := context.Background()
	store := artifact.Store{Root: t.TempDir()}
	publish := func(stage, input string, files map[string]string) artifact.Result {
		t.Helper()
		result, err := store.Publish(ctx, stage, input, strings.Repeat("a", 64), func(dir string) error {
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
		return result
	}
	f := publish("fetch", strings.Repeat("b", 64), map[string]string{"detail.json": `{"data":{"id":"1","docNum":"1","agencyName":"issuer"}}`, "files.json": `[]`})
	excluded := publish("fetch", strings.Repeat("c", 64), map[string]string{"detail.json": `{"data":{"id":"2","docNum":"2"}}`, "files.json": `[]`})
	key := "vn:vbpl:1"
	excludedKey := "vn:vbpl:2"
	fetchItem := InventoryItem{Stage: "fetch", ItemKey: key, State: "done", Path: &f.Path, ArtifactSHA256: &f.SHA256}
	excludedItem := InventoryItem{Stage: "fetch", ItemKey: excludedKey, State: "done", Path: &excluded.Path, ArtifactSHA256: &excluded.SHA256}
	inv := Inventory{ExpectedKeys: []string{key}, Items: []InventoryItem{fetchItem, excludedItem}}
	counts := map[string]int{}
	var mu sync.Mutex
	read := func(store artifact.Store, item InventoryItem) identityResult {
		mu.Lock()
		counts[item.ItemKey]++
		mu.Unlock()
		return readIdentityFetch(store, item)
	}
	_, verified, err := identityInventoryWithFetch(inv, store, identityConcurrency, read)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{key, excludedKey} {
		if counts[k] != 1 {
			t.Fatalf("verified %s %d times", k, counts[k])
		}
	}
	input := relate.PairInputHash(excluded.SHA256, "-")
	r := publish("relate", input, map[string]string{"relations.json": `[{"target_id":"1","target_number":"1","target_anchor":"dieu-1","issuer_code":"I"}]`})
	for _, art := range []artifact.Result{f, excluded} {
		if err := os.Remove(filepath.Join(store.Abs(art.Path), "detail.json")); err != nil {
			t.Fatal(err)
		}
	}
	l := loader{tx: &noneTx{}, artifacts: store, verified: verified, inv: inv, present: map[string]bool{key: true}, items: map[string]map[string]InventoryItem{
		"fetch": {key: fetchItem, excludedKey: excludedItem}, "extract": {excludedKey: {State: "gap"}}, "relate": {excludedKey: {ItemKey: excludedKey, State: "done", Path: &r.Path, ArtifactSHA256: &r.SHA256, InputHash: &input}},
	}, docs: map[string]*loadedDocument{}}
	if err := l.loadDocuments(ctx); err != nil {
		t.Fatalf("verified document was not reused: %v", err)
	}
	l.docs[key].nodes["n"] = 1
	l.docs[key].anchors["n"] = "dieu-1"
	l.gaps = nil
	if err := l.loadRelations(ctx, map[string]string{"I": source.NormalizeIssuer("issuer")}); err != nil {
		t.Fatalf("verified document was not reused: %v", err)
	}
	if l.counts.Relations != 1 || len(l.gaps) != 0 {
		t.Fatalf("relations=%d gaps=%+v", l.counts.Relations, l.gaps)
	}
}
