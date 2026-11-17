package build

import (
	"context"
	"encoding/json"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"github.com/jackc/pgx/v5"
)

func (l *loader) loadSummaries(ctx context.Context, entries []source.HeadingExclusion) error {
	if err := CheckHeadingExclusions(ctx, l.tx, entries); err != nil {
		return err
	}
	excluded := map[string]map[string]bool{}
	for _, e := range entries {
		if excluded[e.Key] == nil {
			excluded[e.Key] = map[string]bool{}
		}
		excluded[e.Key][e.Anchor] = true
	}
	rows, err := l.tx.Query(ctx, `SELECT key,class,status_class,status_basis,is_current,validity_conflict,eff_from,eff_to,text_source,number,title,doc_type,issuer,to_char(issue_date,'YYYY-MM-DD'),text_complete,source_url,EXISTS(SELECT 1 FROM provision p WHERE p.document_id=d.id) FROM document d ORDER BY key COLLATE "C"`)
	if err != nil {
		return fmt.Errorf("query summary documents: %w", err)
	}
	documents, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (summaryDocument, error) {
		var d summaryDocument
		p := &d.Payload
		scanErr := row.Scan(&d.Key, &d.Class, &d.StatusClass, &d.StatusBasis, &d.IsCurrent, &d.ValidityConflict, &d.EffFrom, &d.EffTo, &d.TextSource, &p.Number, &p.Title, &p.Type, &p.Issuer, &p.Issued, &p.TextComplete, &p.SourceURL, &p.HasText)
		return d, scanErr
	})
	if err != nil {
		return fmt.Errorf("read summary documents: %w", err)
	}
	for _, d := range documents {
		id := l.docs[d.Key].id
		if err = l.summaryParts(ctx, id, &d.Payload); err != nil {
			return err
		}
		r, step, e := makeSummary(d, l.gaps, excluded[d.Key])
		if e != nil {
			return e
		}
		switch step {
		case summaryHeadingsDropped:
			l.counts.Summary.HeadingsDropped++
		case summaryOutlineDropped:
			l.counts.Summary.OutlineDropped++
			l.addGap("build", d.Key, "summary_outline_capped", "")
		}
		payload, e := json.Marshal(r.Payload)
		if e != nil {
			return fmt.Errorf("encode summary payload: %w", e)
		}
		_, err = l.tx.Exec(ctx, `INSERT INTO document_summary(document_key,class,projection,indexed,status_class,status_basis,is_current,validity_conflict,eff_from,eff_to,gap_kinds,payload) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, r.Key, r.Class, r.Projection, r.Indexed, r.StatusClass, r.StatusBasis, r.IsCurrent, r.ValidityConflict, r.EffFrom, r.EffTo, r.GapKinds, payload)
		if err != nil {
			return fmt.Errorf("insert summary %s: %w", r.Key, err)
		}
	}
	return nil
}

func (l *loader) summaryParts(ctx context.Context, id int64, p *summaryPayload) error {
	rows, err := l.tx.Query(ctx, `SELECT kind,label,anchor,heading FROM provision WHERE document_id=$1 AND kind IN ('phan','chuong','muc','dieu') ORDER BY preorder`, id)
	if err != nil {
		return fmt.Errorf("query summary outline: %w", err)
	}
	p.Outline, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (summaryOutline, error) {
		var n summaryOutline
		e := row.Scan(&n.Kind, &n.Label, &n.Anchor, &n.Heading)
		return n, e
	})
	if err != nil {
		return fmt.Errorf("read summary outline: %w", err)
	}
	rows, err = l.tx.Query(ctx, `SELECT r.kind,CASE WHEN r.source_document_id=$1 THEN 'outgoing' ELSE 'incoming' END,
 d.key,COALESCE(d.number,CASE WHEN r.source_document_id=$1 THEN r.target_number ELSE r.source_number END),COALESCE(d.title,'')
 FROM relation r LEFT JOIN document d ON d.id=CASE WHEN r.source_document_id=$1 THEN r.target_document_id ELSE r.source_document_id END
 WHERE (r.source_document_id=$1 OR r.target_document_id=$1) AND r.source_provision_id IS NULL AND r.target_provision_id IS NULL AND r.source_node_id IS NULL AND r.target_node_id IS NULL
 ORDER BY r.kind COLLATE "C",r.source_key COLLATE "C",r.target_key COLLATE "C",r.artifact_sha256 COLLATE "C",r.ordinal`, id)
	if err != nil {
		return fmt.Errorf("query summary relations: %w", err)
	}
	p.Relations, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (summaryRelation, error) {
		var r summaryRelation
		e := row.Scan(&r.Kind, &r.Direction, &r.Key, &r.Number, &r.Title)
		return r, e
	})
	if err != nil {
		return fmt.Errorf("read summary relations: %w", err)
	}
	rows, err = l.tx.Query(ctx, summaryNodesSQL, id)
	if err != nil {
		return fmt.Errorf("query summary nodes: %w", err)
	}
	p.Nodes, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (summaryNode, error) {
		var n summaryNode
		e := row.Scan(&n.Code, &n.Name)
		return n, e
	})
	if err != nil {
		return fmt.Errorf("read summary nodes: %w", err)
	}
	return nil
}

const summaryNodesSQL = `WITH RECURSIVE ancestry AS (
 SELECT p.id AS origin,p.id,p.parent_id,0 AS depth FROM provision p WHERE p.document_id=$1
 UNION ALL SELECT a.origin,p.id,p.parent_id,a.depth+1 FROM ancestry a JOIN provision p ON p.id=a.parent_id
), ranked AS (
 SELECT a.origin,t.node_id,t.family_id,t.exclude,dense_rank() OVER (PARTITION BY a.origin,t.family_id ORDER BY a.depth,CASE t.method WHEN 'manual' THEN 0 WHEN 'phapdien_seed' THEN 1 WHEN 'inherited' THEN 2 ELSE 3 END) AS choice
 FROM ancestry a JOIN taxonomy_assignment t ON t.provision_id=a.id
), decided AS (
 SELECT *,bool_or(exclude) OVER (PARTITION BY origin,family_id) AS excluded FROM ranked WHERE choice=1
), selected AS (
 SELECT DISTINCT d.origin,n.code,n.name FROM decided d JOIN taxonomy_node n ON n.id=d.node_id
 WHERE NOT d.excluded AND n.status='active' AND n.provision_count>0
) SELECT code,name FROM selected GROUP BY code,name ORDER BY count(DISTINCT origin) DESC,code COLLATE "C"`
