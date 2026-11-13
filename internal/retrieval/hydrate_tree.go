package retrieval

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func hydrateTrees(ctx context.Context, tx pgx.Tx, ids []int64) (map[int64][]Provision, map[int64][]SourceRelation, error) {
	const treeSQL = `WITH RECURSIVE tree AS (
 SELECT p.id AS group_id,p.id,p.parent_id,p.document_id,p.kind,p.label,p.heading,p.ordinal,p.preorder,p.text,p.anchor,p.node_id,p.locator FROM provision p WHERE p.id=ANY($1::bigint[])
 UNION ALL SELECT t.group_id,p.id,p.parent_id,p.document_id,p.kind,p.label,p.heading,p.ordinal,p.preorder,p.text,p.anchor,p.node_id,p.locator FROM tree t JOIN LATERAL (SELECT * FROM provision WHERE parent_id=t.id AND document_id=t.document_id OFFSET 0) p ON true
 ) SELECT t.group_id,t.id,t.parent_id,t.kind,t.label,t.heading,t.ordinal,t.preorder,t.text,t.anchor,t.node_id,t.locator,d.country,d.key FROM tree t JOIN document d ON d.id=t.document_id ORDER BY t.group_id,t.preorder`
	rows, err := tx.Query(ctx, treeSQL, ids)
	if err != nil {
		return nil, nil, databaseFailure("hydrate descendant provisions failed", err)
	}
	trees := map[int64][]Provision{}
	for rows.Next() {
		var id int64
		var p Provision
		if err = rows.Scan(&id, &p.ID, &p.ParentID, &p.Kind, &p.Label, &p.Heading, &p.Ordinal, &p.Preorder, &p.Text, &p.Identity.Anchor, &p.NodeID, &p.Locator, &p.Identity.Country, &p.Identity.DocumentKey); err != nil {
			rows.Close()
			return nil, nil, databaseFailure("read descendant provision failed", err)
		}
		trees[id] = append(trees[id], p)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, nil, databaseFailure("read descendant provisions failed", err)
	}
	rows.Close()
	const relationSQL = `WITH RECURSIVE tree AS (
 SELECT p.id AS group_id,p.id,p.document_id FROM provision p WHERE p.id=ANY($1::bigint[])
 UNION ALL SELECT t.group_id,p.id,p.document_id FROM tree t JOIN LATERAL (SELECT * FROM provision WHERE parent_id=t.id AND document_id=t.document_id OFFSET 0) p ON true
 ), endpoints AS (SELECT group_id,document_id,array_agg(id) AS ids FROM tree GROUP BY group_id,document_id)
 SELECT e.group_id,r.source_key,r.target_key,r.source_number,r.target_number,sp.anchor,tp.anchor,r.kind,r.raw_label,r.evidence,r.method,r.source_document_id IS NOT NULL,r.target_document_id IS NOT NULL,COALESCE(sd.key,''),COALESCE(td.key,''),r.artifact_sha256,r.ordinal,r.target_node_id, ARRAY(SELECT a.kind FROM (SELECT kind,preorder FROM provision WHERE document_id=tp.document_id AND path @> tp.path OFFSET 0) a ORDER BY a.preorder), ARRAY(SELECT a.label FROM (SELECT label,preorder FROM provision WHERE document_id=tp.document_id AND path @> tp.path OFFSET 0) a ORDER BY a.preorder),COALESCE(r.source_document_id=e.document_id AND r.source_provision_id=ANY(e.ids),false),COALESCE(r.target_document_id=e.document_id AND (r.target_provision_id=ANY(e.ids) OR (gp.path <@ tp.path AND r.kind IN ('repeals','replaces','suspends','amends_supplements') AND r.target_node_id IS NOT NULL AND ('vbpl_references'=ANY(string_to_array(r.method,',')) OR ('text_rule/1'=ANY(string_to_array(r.method,',')) AND r.target_node_id=tp.anchor)))),false)
 FROM endpoints e JOIN LATERAL (
  SELECT incident.id FROM (
   SELECT id FROM relation WHERE (source_document_id=e.document_id AND (source_provision_id IS NULL OR source_provision_id=ANY(e.ids)))
    OR (target_document_id=e.document_id AND (target_provision_id IS NULL OR target_provision_id=ANY(e.ids)))
   ORDER BY kind COLLATE "C",source_key COLLATE "C",COALESCE(target_key,'') COLLATE "C",artifact_sha256 COLLATE "C",ordinal LIMIT $2
  ) incident
  UNION ALL
  SELECT inherited.id FROM (
   SELECT r.id FROM LATERAL (SELECT path FROM provision WHERE id=e.group_id OFFSET 0) g
   JOIN LATERAL (SELECT id,source_key,target_key,artifact_sha256,ordinal,kind,method,target_node_id,target_provision_id,source_document_id,source_provision_id FROM relation
    WHERE target_document_id=e.document_id AND target_provision_id IS NOT NULL AND target_node_id IS NOT NULL
    AND NOT (target_provision_id=ANY(e.ids)) AND kind IN ('repeals','replaces','suspends','amends_supplements')
    AND ('vbpl_references'=ANY(string_to_array(method,',')) OR 'text_rule/1'=ANY(string_to_array(method,',')))
    AND (source_document_id=e.document_id AND (source_provision_id IS NULL OR source_provision_id=ANY(e.ids))) IS NOT TRUE OFFSET 0) r ON true
   JOIN LATERAL (SELECT path,anchor FROM provision WHERE id=r.target_provision_id AND document_id=e.document_id OFFSET 0) t ON true
   WHERE t.path @> g.path AND ('vbpl_references'=ANY(string_to_array(r.method,',')) OR ('text_rule/1'=ANY(string_to_array(r.method,',')) AND r.target_node_id=t.anchor))
   ORDER BY r.kind COLLATE "C",r.source_key COLLATE "C",COALESCE(r.target_key,'') COLLATE "C",r.artifact_sha256 COLLATE "C",r.ordinal LIMIT $2
  ) inherited
 ) selected ON true JOIN LATERAL (SELECT * FROM relation WHERE id=selected.id OFFSET 0) r ON true
 LEFT JOIN document sd ON sd.id=r.source_document_id LEFT JOIN document td ON td.id=r.target_document_id
 LEFT JOIN LATERAL (SELECT anchor FROM provision WHERE id=r.source_provision_id OFFSET 0) sp ON true LEFT JOIN LATERAL (SELECT path FROM provision WHERE id=e.group_id OFFSET 0) gp ON true LEFT JOIN LATERAL (SELECT * FROM provision WHERE id=r.target_provision_id OFFSET 0) tp ON true`
	rows, err = tx.Query(ctx, relationSQL, ids, Recipe2().RelationCap+1)
	if err != nil {
		return nil, nil, databaseFailure("hydrate relations failed", err)
	}
	rels := map[int64][]SourceRelation{}
	for rows.Next() {
		var id int64
		var r SourceRelation
		var kinds, labels []string
		v := &r.Value
		if err = rows.Scan(&id, &v.SourceKey, &v.TargetKey, &v.SourceNumber, &v.TargetNumber, &v.SourceAnchor, &v.TargetAnchor, &v.Kind, &v.RawLabel, &v.Evidence, &v.Method, &v.SourceResolved, &v.TargetResolved, &v.SourceDocKey, &v.TargetDocKey, &r.ArtifactSHA256, &r.Ordinal, &v.TargetNodeID, &kinds, &labels, &r.SourceInHit, &r.TargetInHit); err != nil {
			rows.Close()
			return nil, nil, databaseFailure("read relation failed", err)
		}
		var ancestry []Provision
		for i := range kinds {
			ancestry = append(ancestry, Provision{Kind: kinds[i], Label: labels[i]})
		}
		v.TargetCitation = citation(ancestry)
		rels[id] = append(rels[id], r)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, nil, databaseFailure("read relations failed", err)
	}
	rows.Close()
	return trees, rels, nil
}
