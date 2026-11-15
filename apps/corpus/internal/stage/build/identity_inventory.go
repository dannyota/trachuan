package build

import (
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
	"danny.vn/trachuan/internal/retrieval"
)

func IdentityInventory(inv Inventory, artifacts artifact.Store, anchors map[string]map[string]bool) (evaluation.IdentityInventory, error) {
	return identityInventory(inv, artifacts, anchors, identityConcurrency)
}

func identityFetchError(err error, decoding bool) error {
	if decoding {
		return fmt.Errorf("decode identity fetch: %w", err)
	}
	return fmt.Errorf("identity fetch: %w", err)
}

func identityInventory(inv Inventory, artifacts artifact.Store, anchors map[string]map[string]bool, limit int) (evaluation.IdentityInventory, error) {
	out, _, err := identityInventoryWithFetch(inv, artifacts, anchors, limit, readIdentityFetch)
	return out, err
}

type verifiedFetch struct {
	document fetch.Document
	dir      string
	hash     string
}

func (l *loader) verifiedDocument(item InventoryItem) (verifiedFetch, error) {
	verified, ok := l.verified[item.ItemKey]
	if !ok || item.ArtifactSHA256 == nil || verified.hash != *item.ArtifactSHA256 {
		return verifiedFetch{}, fmt.Errorf("missing verified fetch for %s", item.ItemKey)
	}
	return verified, nil
}

func identityInventoryWithFetch(inv Inventory, artifacts artifact.Store, anchors map[string]map[string]bool, limit int, read func(artifact.Store, InventoryItem) identityResult) (evaluation.IdentityInventory, map[string]verifiedFetch, error) {
	out := evaluation.IdentityInventory{Documents: map[string]evaluation.IdentityDocument{}, Replacements: map[string]string{}}
	verified := make(map[string]verifiedFetch)
	for _, entry := range inv.Membership.Entries {
		if entry.Disposition == "source_replaced" && entry.ReplacementKey != nil {
			out.Replacements[entry.DocumentKey] = *entry.ReplacementKey
		}
	}
	present := inv.Presence()
	fetches := map[string]InventoryItem{}
	extracts := map[string]InventoryItem{}
	for _, item := range inv.Items {
		if item.Stage == "extract" {
			extracts[item.ItemKey] = item
		}
	}
	var fetchItems []InventoryItem
	for _, item := range inv.Items {
		if item.Stage == "fetch" && item.State == "done" {
			fetchItems = append(fetchItems, item)
		}
	}
	fetchPool := newIdentityPool(len(fetchItems), limit, func(i int) identityResult { return read(artifacts, fetchItems[i]) })
	for i, item := range fetchItems {
		result := fetchPool.take(i)
		if result.err != nil {
			fetchPool.stop()
			return out, nil, result.err
		}
		verified[item.ItemKey] = verifiedFetch{document: result.detail, dir: result.dir, hash: *item.ArtifactSHA256}
		if !present[item.ItemKey] {
			continue
		}
		out.Documents[item.ItemKey] = evaluation.IdentityDocument{Number: retrieval.NormalizeNumber(result.detail.Number), Anchors: map[string]bool{}, Texts: map[string]string{}}
		fetches[item.ItemKey] = item
	}
	fetchPool.stop()
	var items []InventoryItem
	for _, item := range inv.Items {
		if (item.Stage == "extract" || item.Stage == "relate") && item.State == "done" && present[item.ItemKey] {
			items = append(items, item)
		}
	}
	pool := newIdentityPool(len(items), limit, func(i int) identityResult {
		item := items[i]
		fetchItem, ok := fetches[item.ItemKey]
		if !ok || fetchItem.ArtifactSHA256 == nil {
			return identityResult{err: fmt.Errorf("missing identity fetch")}
		}
		dir, err := artifactDir(artifacts, item)
		if err != nil {
			return identityResult{err: fmt.Errorf("identity %s: %w", item.Stage, err)}
		}
		if item.Stage == "extract" {
			if item.InputHash == nil {
				return identityResult{err: fmt.Errorf("missing extract input hash")}
			}
			if err := fetch.CheckExtractLineage(dir, *fetchItem.ArtifactSHA256, *item.InputHash); err != nil {
				return identityResult{err: fmt.Errorf("identity extract lineage: %w", err)}
			}
			var nodes []chunk.Node
			if err := readArtifactJSON(dir, "provisions.json", &nodes); err != nil {
				return identityResult{err: err}
			}
			return identityResult{nodes: nodes}
		}
		extract, found := extracts[item.ItemKey]
		if err := checkRelationInput(item, fetchItem, extract, found); err != nil {
			return identityResult{err: err}
		}
		var rows []relationRow
		return identityResult{err: readArtifactJSON(dir, "relations.json", &rows)}
	})
	defer pool.stop()
	for i, item := range items {
		result := pool.take(i)
		if result.err != nil {
			return out, nil, result.err
		}
		if item.Stage == "relate" {
			continue
		}
		doc := out.Documents[item.ItemKey]
		for _, node := range result.nodes {
			if anchors[item.ItemKey][node.Path] {
				doc.Texts[node.Path] = node.Text
			}
			doc.Anchors[node.Path] = true
		}
		out.Documents[item.ItemKey] = doc
	}
	return out, verified, nil
}
