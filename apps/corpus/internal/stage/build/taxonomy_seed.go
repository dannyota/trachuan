package build

import (
	"context"
	"fmt"
)

// TaxonomyStats reports loaded taxonomy rows and missing targets.
type TaxonomyStats struct {
	Nodes                  int64 `json:"nodes"`
	Served                 int64 `json:"served"`
	Seeds                  int64 `json:"seeds"`
	Overrides              int64 `json:"overrides"`
	Excluded               int64 `json:"excluded"`
	MissingArticles        int64 `json:"missing_articles"`
	MissingOverrideTargets int64 `json:"missing_override_targets"`
}

func (l *loader) taxonomyGap(key, kind, detail string, node int64) {
	for _, g := range l.gaps {
		if g.Stage == "build" && g.ItemKey == key && g.Kind == kind && g.Detail == detail {
			return
		}
	}
	l.addGap("build", key, kind, detail)
	l.gaps[len(l.gaps)-1].NodeID = &node
	if kind == "seed_article_missing" {
		l.counts.Taxonomy.MissingArticles++
	} else {
		l.counts.Taxonomy.MissingOverrideTargets++
	}
}

func (l *loader) provision(key, anchor string) int64 {
	d := l.docs[key]
	if d == nil {
		return 0
	}
	for node, a := range d.anchors {
		if a == anchor {
			return d.nodes[node]
		}
	}
	return 0
}

func (l *loader) loadTaxonomy(ctx context.Context, input TaxonomyInput) error {
	if input.version == "" {
		return nil
	}
	nodes, err := l.loadTaxonomyNodes(ctx, input)
	if err != nil {
		return err
	}
	for _, seed := range input.seeds {
		node, ok := nodes[seed.Code]
		if !ok {
			return fmt.Errorf("missing seed node %s", seed.Code)
		}
		key := "vn:vbpl:" + seed.ItemID
		if l.docs[key] == nil {
			continue
		}
		anchor := "dieu-" + seed.Number
		pid := l.provision(key, anchor)
		if pid == 0 {
			l.taxonomyGap(key, "seed_article_missing", seed.Code+" "+anchor, node.id)
			continue
		}
		err = l.batch.queue(ctx, "insert seed", `INSERT INTO taxonomy_assignment(provision_id,node_id,family_id,method,exclude,confidence,taxonomy_version) VALUES($1,$2,$3,'phapdien_seed',false,1,$4) ON CONFLICT (provision_id,node_id) DO NOTHING`, pid, node.id, node.family, input.version)
		if err != nil {
			return err
		}
	}
	for _, o := range input.overrides {
		node, ok := nodes[o.Code]
		if !ok {
			return fmt.Errorf("missing override node %s", o.Code)
		}
		pid := l.provision(o.DocumentKey, o.Anchor)
		if pid == 0 {
			l.taxonomyGap(o.DocumentKey, "override_target_missing", o.Code+" "+o.Anchor, node.id)
			continue
		}
		if err = l.batch.queue(ctx, "replace taxonomy assignment", `DELETE FROM taxonomy_assignment WHERE provision_id=$1 AND family_id=$2 AND method <> 'manual'`, pid, node.family); err != nil {
			return err
		}
		err = l.batch.queue(ctx, "insert override", `INSERT INTO taxonomy_assignment(provision_id,node_id,family_id,method,exclude,confidence,taxonomy_version) VALUES($1,$2,$3,'manual',$4,1,$5)`, pid, node.id, node.family, o.Exclude, input.version)
		if err != nil {
			return err
		}
	}
	if err = l.batch.flush(ctx); err != nil {
		return err
	}
	if _, err = l.tx.Exec(ctx, taxonomyCountsSQL); err != nil {
		return fmt.Errorf("count effective taxonomy: %w", err)
	}
	err = l.tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE method='phapdien_seed'), count(*) FILTER (WHERE method='manual' AND NOT exclude), count(*) FILTER (WHERE exclude) FROM taxonomy_assignment`).Scan(&l.counts.Taxonomy.Seeds, &l.counts.Taxonomy.Overrides, &l.counts.Taxonomy.Excluded)
	if err != nil {
		return fmt.Errorf("count taxonomy assignments: %w", err)
	}
	err = l.tx.QueryRow(ctx, `SELECT count(*) FROM taxonomy_node WHERE status='active' AND provision_count>0`).Scan(&l.counts.Taxonomy.Served)
	if err != nil {
		return fmt.Errorf("count served taxonomy nodes: %w", err)
	}
	return nil
}

const taxonomyCountsSQL = `WITH RECURSIVE ancestry AS (
 SELECT p.id AS provision_id,p.document_id,p.id AS ancestor_id,0 AS depth FROM provision p
 UNION ALL SELECT a.provision_id,a.document_id,p.parent_id,a.depth+1 FROM ancestry a JOIN provision p ON p.id=a.ancestor_id WHERE p.parent_id IS NOT NULL
), matched AS (
 SELECT a.provision_id,a.document_id,t.node_id,t.exclude, min(a.depth) OVER (PARTITION BY a.provision_id,t.family_id) AS nearest,a.depth
 FROM ancestry a JOIN taxonomy_assignment t ON t.provision_id=a.ancestor_id
), counts AS (
 SELECT n.id,count(DISTINCT m.provision_id) AS provisions,count(DISTINCT m.document_id) AS documents
 FROM taxonomy_node n LEFT JOIN taxonomy_node child ON child.path <@ n.path
 LEFT JOIN matched m ON m.node_id=child.id AND m.depth=m.nearest AND NOT m.exclude GROUP BY n.id
) UPDATE taxonomy_node n SET provision_count=c.provisions,document_count=c.documents FROM counts c WHERE c.id=n.id`
