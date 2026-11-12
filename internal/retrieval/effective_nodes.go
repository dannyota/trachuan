package retrieval

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// NodeRef names a served node referenced by a result.
type NodeRef struct {
	Name  string `json:"name"`
	Level int    `json:"level"`
}

// Resolve nearest entries before discarding exclusions or unserved nodes: either
// still blocks inheritance from a more distant provision in the same family.
// Lateral lookups keep the recursive step bounded by each work-table row.
func effectiveNodesCTE(seeds string) string {
	return `WITH RECURSIVE ancestry AS (
 SELECT p.id AS origin,p.id,p.parent_id,0 AS distance FROM (` + seeds + `) p
 UNION ALL SELECT a.origin,p.id,p.parent_id,a.distance+1 FROM ancestry a JOIN LATERAL (SELECT id,parent_id FROM provision WHERE id=a.parent_id OFFSET 0) p ON true
 ), ranked AS (
 SELECT a.origin,t.node_id,t.family_id,t.exclude,dense_rank() OVER (
 PARTITION BY a.origin,t.family_id ORDER BY a.distance,CASE t.method WHEN 'manual' THEN 0 WHEN 'phapdien_seed' THEN 1 ELSE 2 END) AS choice
 FROM ancestry a JOIN LATERAL (SELECT * FROM taxonomy_assignment WHERE provision_id=a.id OFFSET 0) t ON true
 ), decided AS (
 SELECT *,bool_or(exclude) OVER (PARTITION BY origin,family_id) AS excluded FROM ranked WHERE choice=1
 ), effective AS (
 SELECT DISTINCT d.origin,n.code,n.name,n.level FROM decided d JOIN taxonomy_node n ON n.id=d.node_id
 WHERE NOT d.excluded AND n.status='active' AND n.provision_count>0
 ) `
}

func (e *Engine) loadEffectiveNodes(ctx context.Context, tx pgx.Tx, ids []int64) (map[int64][]string, map[string]NodeRef, error) {
	nodes, refs := map[int64][]string{}, map[string]NodeRef{}
	if e.release.SchemaVersion < 5 || len(ids) == 0 {
		return nodes, refs, nil
	}
	rows, err := tx.Query(ctx, effectiveNodesCTE(`SELECT id,parent_id FROM provision WHERE id=ANY($1::bigint[])`)+`SELECT origin,code,name,level FROM effective ORDER BY origin,level DESC,code COLLATE "C"`, ids)
	if err != nil {
		return nil, nil, databaseFailure("query effective nodes", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var code string
		var ref NodeRef
		if err = rows.Scan(&id, &code, &ref.Name, &ref.Level); err != nil {
			return nil, nil, databaseFailure("scan effective nodes", err)
		}
		nodes[id] = append(nodes[id], code)
		refs[code] = ref
	}
	if err = rows.Err(); err != nil {
		return nil, nil, databaseFailure("read effective nodes", err)
	}
	return nodes, refs, nil
}

func (e *Engine) loadDocumentNodes(ctx context.Context, tx pgx.Tx, keys []string) (map[string][]string, error) {
	nodes := map[string][]string{}
	if e.release.SchemaVersion < 5 || len(keys) == 0 {
		return nodes, nil
	}
	rows, err := tx.Query(ctx, effectiveNodesCTE(`SELECT p.id,p.parent_id FROM document d JOIN LATERAL (SELECT id,parent_id FROM provision WHERE document_id=d.id OFFSET 0) p ON true WHERE d.key=ANY($1::text[])`)+`, counts AS (
 SELECT d.key,e.code,count(DISTINCT e.origin) AS provisions FROM effective e JOIN LATERAL (SELECT document_id FROM provision WHERE id=e.origin OFFSET 0) p ON true JOIN document d ON d.id=p.document_id GROUP BY d.key,e.code
 ), numbered AS (
 SELECT *,row_number() OVER (PARTITION BY key ORDER BY provisions DESC,code COLLATE "C") AS position FROM counts
 ) SELECT key,code FROM numbered WHERE position<=10 ORDER BY key,position`, keys)
	if err != nil {
		return nil, databaseFailure("query document nodes", err)
	}
	defer rows.Close()
	for rows.Next() {
		var key, code string
		if err = rows.Scan(&key, &code); err != nil {
			return nil, databaseFailure("scan document nodes", err)
		}
		nodes[key] = append(nodes[key], code)
	}
	if err = rows.Err(); err != nil {
		return nil, databaseFailure("read document nodes", err)
	}
	return nodes, nil
}

func (e *Engine) loadNodeIndex(ctx context.Context, tx pgx.Tx, codes []string) (map[string]NodeRef, error) {
	refs := map[string]NodeRef{}
	if e.release.SchemaVersion < 5 || len(codes) == 0 {
		return refs, nil
	}
	rows, err := tx.Query(ctx, `SELECT code,name,level FROM taxonomy_node WHERE code=ANY($1::text[]) AND status='active' AND provision_count>0`, codes)
	if err != nil {
		return nil, databaseFailure("query node index", err)
	}
	defer rows.Close()
	for rows.Next() {
		var code string
		var ref NodeRef
		if err = rows.Scan(&code, &ref.Name, &ref.Level); err != nil {
			return nil, databaseFailure("scan node index", err)
		}
		refs[code] = ref
	}
	if err = rows.Err(); err != nil {
		return nil, databaseFailure("read node index", err)
	}
	return refs, nil
}

func (e *Engine) nodeIndex(ctx context.Context, codes []string) (map[string]NodeRef, error) {
	if len(codes) == 0 || e.release.SchemaVersion < 5 {
		return map[string]NodeRef{}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, e.deadline())
	defer cancel()
	tx, err := e.metadataTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	refs, err := e.loadNodeIndex(ctx, tx, codes)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, databaseFailure("commit node index", err)
	}
	return refs, nil
}

func documentNodeCodes(docs map[string]DocumentEntry) []string {
	var codes []string
	for _, doc := range docs {
		codes = append(codes, doc.Nodes...)
	}
	return codes
}
