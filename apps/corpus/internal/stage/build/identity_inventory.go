package build

import (
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/internal/retrieval"
)

// IdentityInventory verifies artifacts and extracts document-qualified identities.
func IdentityInventory(inv Inventory, artifacts artifact.Store) (evaluation.IdentityInventory, error) {
	out := evaluation.IdentityInventory{Documents: map[string]evaluation.IdentityDocument{}}
	fetches := map[string]InventoryItem{}
	for _, item := range inv.Items {
		if item.Stage != "fetch" || item.State != "done" {
			continue
		}
		dir, err := artifactDir(artifacts, item)
		if err != nil {
			return out, fmt.Errorf("identity fetch: %w", err)
		}
		var detail fetchDetail
		if err = readArtifactJSON(dir, "detail.json", &detail); err != nil {
			return out, err
		}
		if "vn:vbpl:"+detail.Data.ID != item.ItemKey {
			return out, fmt.Errorf("identity fetch key mismatch")
		}
		out.Documents[item.ItemKey] = evaluation.IdentityDocument{Number: retrieval.NormalizeNumber(detail.Data.DocNum), Anchors: map[string]bool{}, Nodes: map[string]bool{}}
		fetches[item.ItemKey] = item
	}
	for _, item := range inv.Items {
		if (item.Stage != "extract" && item.Stage != "relate") || item.State != "done" {
			continue
		}
		fetch, ok := fetches[item.ItemKey]
		if !ok || fetch.ArtifactSHA256 == nil {
			return out, fmt.Errorf("missing identity fetch")
		}
		if err := checkInputHash(item, *fetch.ArtifactSHA256); err != nil {
			return out, err
		}
		dir, err := artifactDir(artifacts, item)
		if err != nil {
			return out, fmt.Errorf("identity %s: %w", item.Stage, err)
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
