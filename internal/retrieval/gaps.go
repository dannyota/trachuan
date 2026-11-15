package retrieval

import (
	"context"
	"errors"
)

const GapPageSize = 200

func (e *Engine) Gaps(ctx context.Context, kind, node string, after int64, limit int) ([]Gap, int64, error) {
	if after < 0 || limit < 0 || limit > GapPageSize {
		return nil, 0, errors.New("invalid gap pagination")
	}
	if node != "" && !validTaxonomyCode(node) {
		return nil, 0, ErrInvalidRequest
	}
	if limit == 0 {
		limit = GapPageSize
	}
	ctx, cancel := context.WithTimeout(ctx, e.deadline())
	defer cancel()
	tx, err := e.metadataTx(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	query := `SELECT id,kind,COALESCE(document_key,''),message FROM coverage_gap WHERE id>$1 AND ($2='' OR kind=$2) ORDER BY id LIMIT $3`
	args := []any{after, kind, limit + 1}
	if node != "" {
		var refs map[string]NodeRef
		refs, err = e.loadNodeIndex(ctx, tx, []string{node})
		if err != nil {
			return nil, 0, err
		}
		if _, ok := refs[node]; !ok {
			return nil, 0, ErrInvalidRequest
		}
		query = `WITH matched_documents AS (
 SELECT DISTINCT p.document_id FROM taxonomy_node n JOIN taxonomy_assignment a ON a.node_id=n.id
 JOIN provision p ON p.id=a.provision_id JOIN coverage_gap g ON g.document_id=p.document_id
 WHERE n.status='active' AND n.provision_count>0 AND ` + nodeCodePredicateSQL("n.code") + `
 AND g.id>$1 AND ($2='' OR g.kind=$2)
 AND NOT EXISTS (
  SELECT 1 FROM taxonomy_assignment t WHERE t.provision_id=a.provision_id AND t.family_id=a.family_id
  AND (` + assignmentMethodRankSQL("t") + ` < ` + assignmentMethodRankSQL("a") + `
   OR (` + assignmentMethodRankSQL("t") + ` = ` + assignmentMethodRankSQL("a") + ` AND t.exclude))
 )
 ), matching_ids AS (
 SELECT g.id FROM taxonomy_node n JOIN coverage_gap g ON g.node_id=n.id WHERE ` + nodeCodePredicateSQL("n.code") + ` AND g.id>$1 AND ($2='' OR g.kind=$2)
 UNION SELECT g.id FROM matched_documents d JOIN coverage_gap g ON g.document_id=d.document_id WHERE g.id>$1 AND ($2='' OR g.kind=$2)
 ) SELECT g.id,g.kind,COALESCE(g.document_key,''),g.message FROM matching_ids m JOIN coverage_gap g ON g.id=m.id ORDER BY g.id LIMIT $3`
		args = append(args, node)
	}
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, databaseFailure("query gaps", err)
	}
	page := make([]Gap, 0, limit)
	var last, next int64
	for rows.Next() {
		var id int64
		var gap Gap
		if err = rows.Scan(&id, &gap.Kind, &gap.DocumentKey, &gap.Message); err != nil {
			rows.Close()
			return nil, 0, databaseFailure("scan gap", err)
		}
		if len(page) == limit {
			next = last
			break
		}
		page = append(page, gap)
		last = id
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, 0, databaseFailure("read gaps", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, 0, databaseFailure("commit gaps", err)
	}
	return page, next, nil
}
