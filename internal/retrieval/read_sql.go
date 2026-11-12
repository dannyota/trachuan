package retrieval

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

func (e *Engine) readDocument(ctx context.Context, tx pgx.Tx, item ReadItem) (int64, string, DocumentEntry, Document, string, error) {
	key, _, _ := ParseHandle(item.Handle)
	if item.Number != "" {
		rows, err := tx.Query(ctx, `SELECT key FROM document WHERE number_norm=$1 ORDER BY key LIMIT 2`, NormalizeNumber(item.Number))
		if err != nil {
			return 0, "", DocumentEntry{}, Document{}, "", databaseFailure("query read number", err)
		}
		var keys []string
		for rows.Next() {
			var k string
			if err = rows.Scan(&k); err != nil {
				rows.Close()
				return 0, "", DocumentEntry{}, Document{}, "", databaseFailure("scan read number", err)
			}
			keys = append(keys, k)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return 0, "", DocumentEntry{}, Document{}, "", databaseFailure("read number", err)
		}
		if len(keys) == 0 {
			return 0, "", DocumentEntry{}, Document{}, "not_found", nil
		}
		if len(keys) > 1 {
			return 0, "", DocumentEntry{}, Document{}, "ambiguous_number", nil
		}
		key = keys[0]
	}
	var id int64
	var doc DocumentEntry
	var issued, from, to *time.Time
	var source Document
	err := tx.QueryRow(ctx, `SELECT id,number,title,doc_type,issuer,issue_date,eff_from,eff_to,COALESCE(source_url,''),status_class,status_basis,is_current,validity_conflict,text_source,text_complete,fetch_sha256 FROM document WHERE key=$1`, key).Scan(&id, &doc.Number, &doc.Title, &doc.Type, &doc.Issuer, &issued, &from, &to, &doc.SourceURL, &source.StatusClass, &source.StatusBasis, &source.IsCurrent, &source.ValidityConflict, &doc.TextSource, &doc.TextComplete, &source.FetchSHA256)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, key, doc, Document{}, "not_found", nil
	}
	if err != nil {
		return 0, key, doc, Document{}, "", databaseFailure("query read document", err)
	}
	doc.Issued, doc.EffectiveFrom, doc.EffectiveTo = dateValue(issued), dateValue(from), dateValue(to)
	source.EffectiveFrom, source.EffectiveTo = from, to
	rows, err := tx.Query(ctx, documentCountsSQL, []string{key})
	if err != nil {
		return 0, key, doc, Document{}, "", databaseFailure("query read relation counts", err)
	}
	for rows.Next() {
		var docKey, dir, kind string
		var count int
		if err = rows.Scan(&docKey, &dir, &kind, &count); err != nil {
			rows.Close()
			return 0, key, doc, Document{}, "", databaseFailure("scan read relation count", err)
		}
		if doc.RelatedCounts == nil {
			doc.RelatedCounts = make(map[string]int)
		}
		doc.RelatedCounts[dir+":"+kind] = count
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, key, doc, Document{}, "", databaseFailure("read relation counts", err)
	}
	rows, err = tx.Query(ctx, documentPointersSQL, []string{key}, pointerKinds(), PointersPerDocument)
	if err != nil {
		return 0, key, doc, Document{}, "", databaseFailure("query read pointers", err)
	}
	for rows.Next() {
		var docKey string
		var p Pointer
		if err = rows.Scan(&docKey, &p.Dir, &p.Kind, &p.Key, &p.Number, &p.InRelease); err != nil {
			rows.Close()
			return 0, key, doc, Document{}, "", databaseFailure("scan read pointer", err)
		}
		doc.Related = append(doc.Related, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, key, doc, Document{}, "", databaseFailure("read pointers", err)
	}
	nodes, err := e.loadDocumentNodes(ctx, tx, []string{key})
	if err != nil {
		return 0, key, doc, Document{}, "", err
	}
	doc.Nodes = append([]string(nil), nodes[key]...)
	return id, key, doc, source, "", nil
}

func readNodes(ctx context.Context, tx pgx.Tx, id int64, key string) ([]Provision, error) {
	rows, err := tx.Query(ctx, `SELECT id,parent_id,kind,label,heading,ordinal,preorder,text,anchor,node_id,locator FROM provision WHERE document_id=$1 ORDER BY preorder`, id)
	if err != nil {
		return nil, databaseFailure("query read provisions", err)
	}
	return scanReadNodes(rows, key)
}

func readSubtree(ctx context.Context, tx pgx.Tx, id int64, key, anchor string) ([]Provision, error) {
	const sql = `WITH RECURSIVE tree AS (
 SELECT p.* FROM provision p WHERE document_id=$1 AND anchor=$2
 UNION ALL SELECT p.* FROM tree t JOIN LATERAL
 (SELECT * FROM provision WHERE parent_id=t.id AND document_id=t.document_id OFFSET 0) p ON true
 ) SELECT id,parent_id,kind,label,heading,ordinal,preorder,text,anchor,node_id,locator FROM tree ORDER BY preorder`
	rows, err := tx.Query(ctx, sql, id, anchor)
	if err != nil {
		return nil, databaseFailure("query read subtree", err)
	}
	return scanReadNodes(rows, key)
}

func scanReadNodes(rows pgx.Rows, key string) ([]Provision, error) {
	defer rows.Close()
	var nodes []Provision
	for rows.Next() {
		p := Provision{Identity: Identity{Country: "vn", DocumentKey: key}}
		if err := rows.Scan(&p.ID, &p.ParentID, &p.Kind, &p.Label, &p.Heading, &p.Ordinal, &p.Preorder, &p.Text, &p.Identity.Anchor, &p.NodeID, &p.Locator); err != nil {
			return nil, databaseFailure("scan read provision", err)
		}
		nodes = append(nodes, p)
	}
	if err := rows.Err(); err != nil {
		return nil, databaseFailure("read provisions", err)
	}
	return nodes, nil
}

// The shared incident CTE preserves pointer ordering and both self directions.
const readRelationsSQL = `WITH incident AS (` + incidentRelationsSQL + `), scoped AS (
 SELECT i.*,r.raw_label,r.evidence FROM incident i JOIN relation r
 ON r.source_key=i.source_key AND r.artifact_sha256=i.artifact_sha256 AND r.ordinal=i.ordinal
 WHERE NOT $3 OR (i.dir='in' AND (r.target_provision_id IS NULL OR r.target_provision_id=ANY($4::bigint[])))
 OR (i.dir='out' AND (r.source_provision_id IS NULL OR r.source_provision_id=ANY($4::bigint[])))
 OR (i.dir='in' AND r.target_provision_id=ANY($8::bigint[]) AND r.target_node_id IS NOT NULL AND r.kind IN ('repeals','replaces','suspends','amends_supplements') AND EXISTS (SELECT 1 FROM provision tp WHERE tp.id=r.target_provision_id AND ('vbpl_references'=ANY(string_to_array(r.method,',')) OR ('text_rule/1'=ANY(string_to_array(r.method,',')) AND r.target_node_id=tp.anchor))))
 ) SELECT dir,kind,COALESCE(handle,other_key,''),other_number,handle IS NOT NULL,raw_label,evidence,
 count(*) OVER (),bool_or(char_length(evidence)>$7) OVER () FROM scoped ORDER BY
 COALESCE(array_position($2::text[],kind),cardinality($2::text[])+1),
 dir COLLATE "C",issue_date DESC NULLS LAST,other_number COLLATE "C",
 source_key COLLATE "C",artifact_sha256 COLLATE "C",ordinal LIMIT $5 OFFSET $6`

func readRelations(ctx context.Context, tx pgx.Tx, key string, ids, ancestors []int64, provision bool, offset, evidenceCap int) (readSequence, error) {
	seq := readSequence{offset: offset, complete: true}
	rows, err := tx.Query(ctx, readRelationsSQL, []string{key}, pointerKinds(), provision, ids, RelationPageSize, offset, evidenceCap, ancestors)
	if err != nil {
		return seq, databaseFailure("query read relations", err)
	}
	defer rows.Close()
	for rows.Next() {
		var r ListedRelation
		var total int
		var cut bool
		if err = rows.Scan(&r.Dir, &r.Kind, &r.Key, &r.Number, &r.InRelease, &r.RawLabel, &r.Evidence, &total, &cut); err != nil {
			return seq, databaseFailure("scan read relation", err)
		}
		evidence := []rune(r.Evidence)
		r.EvidenceComplete = len(evidence) <= evidenceCap
		if !r.EvidenceComplete {
			r.Evidence = string(evidence[:evidenceCap])
		}
		seq.units = append(seq.units, Unit{Kind: "relation", Relation: &r})
		seq.more = offset+len(seq.units) < total
		seq.complete = !cut
	}
	if err = rows.Err(); err != nil {
		return seq, databaseFailure("read relations", err)
	}
	return seq, nil
}
