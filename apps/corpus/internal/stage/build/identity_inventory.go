package build

import (
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
	"danny.vn/trachuan/internal/retrieval"
)

// IdentityInventory verifies artifacts and extracts document-qualified identities.
func IdentityInventory(inv Inventory, artifacts artifact.Store) (evaluation.IdentityInventory, error) {
	out := evaluation.IdentityInventory{Documents: map[string]evaluation.IdentityDocument{}, Replacements: map[string]string{}}
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
	for _, item := range inv.Items {
		if item.Stage != "fetch" || item.State != "done" {
			continue
		}
		dir, err := artifactDir(artifacts, item)
		if err != nil {
			return out, fmt.Errorf("identity fetch: %w", err)
		}
		detail, err := fetch.ReadDocument(dir, item.ItemKey)
		if err != nil {
			return out, fmt.Errorf("decode identity fetch: %w", err)
		}
		if !present[item.ItemKey] {
			continue
		}
		out.Documents[item.ItemKey] = evaluation.IdentityDocument{Number: retrieval.NormalizeNumber(detail.Number), Anchors: map[string]bool{}, Nodes: map[string]bool{}}
		fetches[item.ItemKey] = item
	}
	for _, item := range inv.Items {
		if (item.Stage != "extract" && item.Stage != "relate") || item.State != "done" {
			continue
		}
		fetchItem, ok := fetches[item.ItemKey]
		if !present[item.ItemKey] {
			continue
		}
		if !ok || fetchItem.ArtifactSHA256 == nil {
			return out, fmt.Errorf("missing identity fetch")
		}
		dir, err := artifactDir(artifacts, item)
		if err != nil {
			return out, fmt.Errorf("identity %s: %w", item.Stage, err)
		}
		if item.Stage == "extract" {
			if item.InputHash == nil {
				return out, fmt.Errorf("missing extract input hash")
			}
			if err = fetch.CheckExtractLineage(dir, *fetchItem.ArtifactSHA256, *item.InputHash); err != nil {
				return out, fmt.Errorf("identity extract lineage: %w", err)
			}
		} else {
			extract, found := extracts[item.ItemKey]
			if err = checkRelationInput(item, fetchItem, extract, found); err != nil {
				return out, err
			}
		}
		if item.Stage == "relate" {
			var rows []relationRow
			if err = readArtifactJSON(dir, "relations.json", &rows); err != nil {
				return out, err
			}
			continue
		}
		var nodes []chunk.Node
		if err = readArtifactJSON(dir, "provisions.json", &nodes); err != nil {
			return out, err
		}
		doc := out.Documents[item.ItemKey]
		for _, node := range nodes {
			doc.Nodes[node.NodeID] = true
			doc.Anchors[node.Path] = true
		}
		out.Documents[item.ItemKey] = doc
	}
	return out, nil
}
