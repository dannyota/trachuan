package build

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func validateTaxonomy(ctx context.Context, db *pgx.Conn, record BuildRecord) error {
	if record.Taxonomy == nil {
		return fmt.Errorf("missing taxonomy stats: %w", errInvalidDatabase)
	}
	var actual TaxonomyStats
	err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM taxonomy_node), (SELECT count(*) FROM taxonomy_node WHERE status='active' AND provision_count>0), (SELECT count(*) FROM taxonomy_assignment WHERE method='phapdien_seed'), (SELECT count(*) FROM taxonomy_assignment WHERE method='manual' AND NOT exclude), (SELECT count(*) FROM taxonomy_assignment WHERE exclude), (SELECT count(*) FROM coverage_gap WHERE stage='build' AND kind='seed_article_missing'), (SELECT count(*) FROM coverage_gap WHERE stage='build' AND kind='override_target_missing')`).Scan(&actual.Nodes, &actual.Served, &actual.Seeds, &actual.Overrides, &actual.Excluded, &actual.MissingArticles, &actual.MissingOverrideTargets)
	if err != nil {
		return fmt.Errorf("read taxonomy stats: %w", err)
	}
	if actual != *record.Taxonomy {
		return fmt.Errorf("taxonomy stats mismatch: %w", errInvalidDatabase)
	}
	const checks = `SELECT
 NOT EXISTS (SELECT 1 FROM taxonomy_node n LEFT JOIN taxonomy_node p ON p.id=n.parent_id WHERE (p.id IS NULL AND (n.parent_id IS NOT NULL OR n.level<>1 OR n.path<>text2ltree('n'||n.id))) OR (p.id IS NOT NULL AND (n.level<>p.level+1 OR n.path<>p.path||text2ltree('n'||n.id))))
 AND NOT EXISTS (SELECT 1 FROM (SELECT ordinal,row_number() OVER (PARTITION BY parent_id ORDER BY ordinal) AS expected FROM taxonomy_node) o WHERE ordinal<>expected)
 AND NOT EXISTS (SELECT 1 FROM taxonomy_assignment a JOIN taxonomy_node n ON n.id=a.node_id LEFT JOIN taxonomy_node f ON f.id=a.family_id WHERE f.level<>2 OR NOT (n.path <@ f.path) OR (a.exclude AND a.method<>'manual') OR a.taxonomy_version<>$1)
 AND NOT EXISTS (SELECT 1 FROM taxonomy_assignment GROUP BY provision_id,family_id HAVING bool_or(method='manual') AND bool_or(method<>'manual'))`
	var ok bool
	if err := db.QueryRow(ctx, checks, record.TaxonomyVersion).Scan(&ok); err != nil {
		return fmt.Errorf("validate taxonomy structure: %w", err)
	}
	if !ok {
		return fmt.Errorf("invalid taxonomy structure: %w", errInvalidDatabase)
	}
	const counts = `WITH RECURSIVE expanded AS (
 SELECT a.provision_id AS source,p.id AS target,p.document_id,a.family_id,a.node_id,a.exclude,0 AS depth FROM taxonomy_assignment a JOIN provision p ON p.id=a.provision_id
 UNION ALL SELECT e.source,p.id,p.document_id,e.family_id,e.node_id,e.exclude,e.depth+1 FROM expanded e JOIN provision p ON p.parent_id=e.target
), nearest AS (
 SELECT *,min(depth) OVER (PARTITION BY target,family_id) AS best FROM expanded
), effective AS (SELECT DISTINCT target,document_id,node_id FROM nearest WHERE depth=best AND NOT exclude), computed AS (
 SELECT n.id,count(DISTINCT e.target) AS provisions,count(DISTINCT e.document_id) AS documents FROM taxonomy_node n LEFT JOIN taxonomy_node c ON c.path <@ n.path LEFT JOIN effective e ON e.node_id=c.id GROUP BY n.id
) SELECT NOT EXISTS (SELECT 1 FROM taxonomy_node n JOIN computed c ON c.id=n.id WHERE n.provision_count<>c.provisions OR n.document_count<>c.documents)`
	if err := db.QueryRow(ctx, counts).Scan(&ok); err != nil {
		return fmt.Errorf("validate taxonomy counts: %w", err)
	}
	if !ok {
		return fmt.Errorf("invalid taxonomy counts: %w", errInvalidDatabase)
	}
	return nil
}
