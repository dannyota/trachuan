package build

import (
	"context"
	"fmt"
	"sort"
)

func (l *loader) inheritance(ctx context.Context, input TaxonomyInput, nodes map[string]taxonomyNode) (inheritanceResult, error) {
	byID := map[int64]taxonomyNode{}
	for _, n := range nodes {
		byID[n.id] = n
	}
	seeds := map[int64][]int64{}
	for _, seed := range input.seeds {
		n, ok := nodes[seed.Code]
		if !ok {
			return inheritanceResult{}, fmt.Errorf("missing seed node %s", seed.Code)
		}
		if id := l.provision("vn:vbpl:"+seed.ItemID, "dieu-"+seed.Number); id != 0 {
			seeds[id] = append(seeds[id], n.id)
		}
	}
	rows, err := l.tx.Query(ctx, `SELECT p.id,p.document_id,p.anchor,p.heading,d.is_consolidated FROM provision p JOIN document d ON d.id=p.document_id WHERE p.kind='dieu'`)
	if err != nil {
		return inheritanceResult{}, fmt.Errorf("read inheritance articles: %w", err)
	}
	var articles []inheritanceArticle
	for rows.Next() {
		var a inheritanceArticle
		if err = rows.Scan(&a.id, &a.doc, &a.anchor, &a.heading, &a.consolidated); err != nil {
			rows.Close()
			return inheritanceResult{}, fmt.Errorf("scan inheritance article: %w", err)
		}
		a.seeds = seeds[a.id]
		articles = append(articles, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return inheritanceResult{}, fmt.Errorf("read inheritance articles: %w", err)
	}
	rows, err = l.tx.Query(ctx, `SELECT DISTINCT source_document_id,target_document_id FROM relation WHERE kind='consolidates' AND source_document_id IS NOT NULL AND target_document_id IS NOT NULL`)
	if err != nil {
		return inheritanceResult{}, fmt.Errorf("read inheritance relations: %w", err)
	}
	sources := map[int64][]int64{}
	for rows.Next() {
		var source, target int64
		if err = rows.Scan(&source, &target); err != nil {
			rows.Close()
			return inheritanceResult{}, fmt.Errorf("scan inheritance relation: %w", err)
		}
		sources[source] = append(sources[source], target)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return inheritanceResult{}, fmt.Errorf("read inheritance relations: %w", err)
	}
	return inheritArticles(articles, sources, byID), nil
}

func (l *loader) insertInheritance(ctx context.Context, result inheritanceResult, nodes map[string]taxonomyNode, version string) error {
	if result.gate.Status != "pass" {
		return nil
	}
	byID := map[int64]taxonomyNode{}
	for _, n := range nodes {
		byID[n.id] = n
	}
	ids := make([]int64, 0, len(result.assignments))
	for id := range result.assignments {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		nodeIDs := make([]int64, 0, len(result.assignments[id]))
		for node := range result.assignments[id] {
			nodeIDs = append(nodeIDs, node)
		}
		sort.Slice(nodeIDs, func(i, j int) bool { return nodeIDs[i] < nodeIDs[j] })
		for _, node := range nodeIDs {
			if _, err := l.tx.Exec(ctx, `INSERT INTO taxonomy_assignment(provision_id,node_id,family_id,method,exclude,confidence,taxonomy_version) VALUES($1,$2,$3,'inherited',false,0.9,$4) ON CONFLICT (provision_id,node_id) DO NOTHING`, id, node, byID[node].family, version); err != nil {
				return fmt.Errorf("insert inherited assignment: %w", err)
			}
		}
	}
	return nil
}
