package cli

import (
	"context"
	"fmt"
	"os"

	"danny.vn/trachuan/apps/corpus/internal/classify"
	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
	"github.com/jackc/pgx/v5/pgxpool"
)

func loadDrawEvidence(ctx context.Context, cc, rel string) (drawEvidence, error) {
	var e drawEvidence
	name, err := build.DatabaseName(cc, rel)
	if err != nil {
		return e, err
	}
	url := os.Getenv("CORPUS_BUILDER_DATABASE_URL")
	if url == "" {
		return e, fmt.Errorf("CORPUS_BUILDER_DATABASE_URL is required")
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return e, fmt.Errorf("invalid database configuration")
	}
	cfg.ConnConfig.Database = name
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return e, fmt.Errorf("open release database: %w", err)
	}
	defer db.Close()
	if err = db.QueryRow(ctx, `SELECT taxonomy_version FROM release WHERE singleton AND country=$1 AND rel=$2`, cc, rel).Scan(&e.taxonomyHash); err != nil {
		return e, fmt.Errorf("read release identity: %w", err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM taxonomy_assignment WHERE method='manual'`).Scan(&e.manualCount); err != nil {
		return e, fmt.Errorf("count manual assignments: %w", err)
	}
	if e.manualCount > 0 {
		return e, nil
	}
	if e.nodes, err = classify.LoadNodes(ctx, db); err != nil {
		return e, err
	}
	for _, n := range e.nodes {
		if n.Active {
			e.tree = append(e.tree, taxonomy.PacketNode{Code: n.Code, ParentCode: n.Parent, Level: n.Level, Name: n.Name})
		}
	}
	rows, err := db.Query(ctx, `SELECT p.id,d.id,d.key,p.anchor,p.heading,d.is_consolidated,p.text,d.number,d.title,d.class,COALESCE(d.issue_date::text,''),COALESCE(array_agg(n.code ORDER BY n.code) FILTER (WHERE a.method='phapdien_seed'),ARRAY[]::text[]) FROM provision p JOIN document d ON d.id=p.document_id LEFT JOIN taxonomy_assignment a ON a.provision_id=p.id AND a.method='phapdien_seed' LEFT JOIN taxonomy_node n ON n.id=a.node_id WHERE p.kind='dieu' GROUP BY p.id,d.id ORDER BY d.key,p.anchor`)
	if err != nil {
		return e, fmt.Errorf("read draw articles: %w", err)
	}
	var all []taxonomy.InheritanceArticle
	for rows.Next() {
		var a taxonomy.InheritanceArticle
		var draw taxonomy.DrawArticle
		var class string
		if err = rows.Scan(&a.ID, &a.DocumentID, &draw.Key, &a.Anchor, &a.Heading, &a.Consolidated, &draw.Text, &draw.Number, &draw.Title, &class, &draw.IssueDate, &a.Seeds); err != nil {
			rows.Close()
			return e, fmt.Errorf("scan draw article: %w", err)
		}
		draw.Anchor = a.Anchor
		draw.Primary = class == "primary"
		draw.Seeded = len(a.Seeds) > 0
		all = append(all, a)
		e.articles = append(e.articles, draw)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return e, fmt.Errorf("read draw articles: %w", err)
	}
	rows, err = db.Query(ctx, `SELECT DISTINCT source_document_id,target_document_id FROM relation WHERE kind='consolidates' AND source_document_id IS NOT NULL AND target_document_id IS NOT NULL`)
	if err != nil {
		return e, fmt.Errorf("read consolidation relations: %w", err)
	}
	sources := map[int64][]int64{}
	for rows.Next() {
		var source, target int64
		if err = rows.Scan(&source, &target); err != nil {
			rows.Close()
			return e, fmt.Errorf("scan consolidation relation: %w", err)
		}
		sources[source] = append(sources[source], target)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return e, fmt.Errorf("read consolidation relations: %w", err)
	}
	inherited := taxonomy.InheritanceCandidates(all, sources)
	for i, a := range all {
		e.articles[i].Inherited = len(inherited[a.ID]) > 0
	}
	return e, nil
}
