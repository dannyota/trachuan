package retrieval

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type warningSet struct {
	warnings []Warning
	omitted  int
}

// The eligible relation set is built once per document, before matching units.
const warningSQL = `WITH units AS MATERIALIZED (
 SELECT p.id,p.document_id,p.path FROM provision p WHERE p.id=ANY($1::bigint[])
), docs AS (SELECT DISTINCT document_id FROM units), eligible AS MATERIALIZED (
 SELECT d.document_id,r.source_key,r.target_key,r.source_number,r.source_document_id,r.source_provision_id,
 r.target_provision_id,r.target_node_id,r.kind,r.method,r.artifact_sha256,r.ordinal,tp.anchor,tp.path
 FROM docs d JOIN LATERAL (
  SELECT source_key,target_key,source_number,source_document_id,source_provision_id,target_provision_id,target_node_id,kind,method,artifact_sha256,ordinal
  FROM relation WHERE target_document_id=d.document_id AND target_provision_id IS NOT NULL AND target_node_id IS NOT NULL
  AND kind IN ('repeals','replaces','suspends','amends_supplements')
  AND ('text_rule/1'=ANY(string_to_array(method,',')) OR 'vbpl_references'=ANY(string_to_array(method,','))) OFFSET 0
 ) r ON true JOIN LATERAL (SELECT anchor,path FROM provision WHERE id=r.target_provision_id AND document_id=d.document_id OFFSET 0) tp ON true
)
SELECT u.id,w.source_key,w.source_number,w.kind,w.method,w.target_node_id,w.anchor,
 COALESCE(sd.key,''),sd.id IS NOT NULL,sp.anchor,td.key,
 ARRAY(SELECT a.kind FROM (SELECT kind,preorder FROM provision WHERE document_id=u.document_id AND path @> w.path OFFSET 0) a ORDER BY a.preorder),
 ARRAY(SELECT a.label FROM (SELECT label,preorder FROM provision WHERE document_id=u.document_id AND path @> w.path OFFSET 0) a ORDER BY a.preorder),w.total
FROM units u JOIN document td ON td.id=u.document_id
JOIN LATERAL (
 SELECT e.*,count(*) OVER () AS total FROM eligible e
 WHERE e.document_id=u.document_id AND (u.path @> e.path OR (u.path <@ e.path AND
 ('vbpl_references'=ANY(string_to_array(e.method,',')) OR ('text_rule/1'=ANY(string_to_array(e.method,',')) AND e.target_node_id=e.anchor))))
 ORDER BY e.kind COLLATE "C",e.source_key COLLATE "C",COALESCE(e.target_key,'') COLLATE "C",e.artifact_sha256 COLLATE "C",e.ordinal LIMIT 10
) w ON true
LEFT JOIN document sd ON sd.id=w.source_document_id
LEFT JOIN LATERAL (SELECT anchor FROM provision WHERE id=w.source_provision_id OFFSET 0) sp ON true
ORDER BY u.id,w.kind COLLATE "C",w.source_key COLLATE "C",COALESCE(w.target_key,'') COLLATE "C",w.artifact_sha256 COLLATE "C",w.ordinal`

func loadWarnings(ctx context.Context, tx pgx.Tx, ids []int64) (map[int64]warningSet, error) {
	result := make(map[int64]warningSet)
	if len(ids) == 0 {
		return result, nil
	}
	rows, err := tx.Query(ctx, warningSQL, ids)
	if err != nil {
		return nil, databaseFailure("query warnings", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var r Relation
		var kinds, labels []string
		var total int
		if err = rows.Scan(&id, &r.SourceKey, &r.SourceNumber, &r.Kind, &r.Method, &r.TargetNodeID, &r.TargetAnchor, &r.SourceDocKey, &r.SourceResolved, &r.SourceAnchor, &r.TargetDocKey, &kinds, &labels, &total); err != nil {
			return nil, databaseFailure("scan warning", err)
		}
		var ancestry []Provision
		for i := range kinds {
			ancestry = append(ancestry, Provision{Kind: kinds[i], Label: labels[i]})
		}
		r.TargetResolved = true
		r.TargetCitation = citation(ancestry)
		w, ok := warning(r)
		if !ok {
			continue
		}
		set := result[id]
		set.warnings = append(set.warnings, w)
		if total > 10 {
			set.omitted = total - 10
		}
		result[id] = set
	}
	if err = rows.Err(); err != nil {
		return nil, databaseFailure("read warnings", err)
	}
	return result, nil
}
