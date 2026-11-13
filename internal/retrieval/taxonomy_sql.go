package retrieval

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func loadTaxonomy(ctx context.Context, tx pgx.Tx) ([]taxonomyRow, error) {
	rows, err := tx.Query(ctx, `WITH RECURSIVE tree AS (
 SELECT id,parent_id,code,name,description,level,origin,provision_count,document_count,ARRAY[ordinal] AS ordering FROM taxonomy_node WHERE parent_id IS NULL AND status='active' AND provision_count>0
 UNION ALL SELECT n.id,n.parent_id,n.code,n.name,n.description,n.level,n.origin,n.provision_count,n.document_count,t.ordering||n.ordinal FROM taxonomy_node n JOIN tree t ON n.parent_id=t.id WHERE n.status='active' AND n.provision_count>0
 ) SELECT id,coalesce(parent_id,0),code,name,description,level,origin,provision_count,document_count FROM tree ORDER BY ordering`)
	if err != nil {
		return nil, databaseFailure("query taxonomy", err)
	}
	defer rows.Close()
	result := []taxonomyRow{}
	for rows.Next() {
		var r taxonomyRow
		if err = rows.Scan(&r.id, &r.parent, &r.entry.Code, &r.entry.Name, &r.entry.Description, &r.entry.Level, &r.entry.Origin, &r.entry.Provisions, &r.entry.Documents); err != nil {
			return nil, databaseFailure("scan taxonomy", err)
		}
		r.entry.Children = []NodeEntry{}
		result = append(result, r)
	}
	if err = rows.Err(); err != nil {
		return nil, databaseFailure("read taxonomy", err)
	}
	return result, nil
}

func taxonomyDocuments(ctx context.Context, tx pgx.Tx, id int64) ([]NodeDocument, error) {
	rows, err := tx.Query(ctx, `WITH RECURSIVE subtree AS (
 SELECT id,level FROM taxonomy_node WHERE id=$1 AND status='active' AND provision_count>0
 UNION ALL SELECT n.id,n.level FROM taxonomy_node n JOIN subtree s ON n.parent_id=s.id WHERE n.status='active' AND n.provision_count>0
 ), families AS (
 SELECT id FROM subtree WHERE level=2
 UNION SELECT f.id FROM taxonomy_node f JOIN taxonomy_node target ON target.id=$1 AND f.level=2 AND f.path @> target.path
 ), expanded AS (
 SELECT a.provision_id,p0.document_id,a.node_id,a.family_id,a.exclude,0 AS distance FROM taxonomy_assignment a JOIN families f ON f.id=a.family_id JOIN provision p0 ON p0.id=a.provision_id
 UNION ALL SELECT p.id,p.document_id,e.node_id,e.family_id,e.exclude,e.distance+1 FROM expanded e JOIN LATERAL (SELECT id,document_id FROM provision WHERE document_id=e.document_id AND parent_id=e.provision_id OFFSET 0) p ON true
 ), nearest AS (
 SELECT provision_id,family_id,min(distance) AS distance FROM expanded GROUP BY provision_id,family_id
 ), effective AS (
 SELECT DISTINCT e.provision_id FROM expanded e JOIN nearest n USING (provision_id,family_id,distance) JOIN subtree s ON s.id=e.node_id WHERE NOT e.exclude
 ) SELECT d.key,d.number,d.title,d.doc_type,d.status_class,d.is_current,count(DISTINCT p.id) AS provisions FROM effective e JOIN provision p ON p.id=e.provision_id JOIN document d ON d.id=p.document_id GROUP BY d.id ORDER BY provisions DESC,d.key ASC LIMIT 20`, id)
	if err != nil {
		return nil, databaseFailure("query taxonomy documents", err)
	}
	defer rows.Close()
	result := []NodeDocument{}
	for rows.Next() {
		var d NodeDocument
		if err = rows.Scan(&d.Key, &d.Number, &d.Title, &d.Type, &d.StatusClass, &d.IsCurrent, &d.Provisions); err != nil {
			return nil, databaseFailure("scan taxonomy document", err)
		}
		result = append(result, d)
	}
	if err = rows.Err(); err != nil {
		return nil, databaseFailure("read taxonomy documents", err)
	}
	return result, nil
}
