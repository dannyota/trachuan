package knowledge

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

type SQLReader struct{ Q Querier }

func collect[T any](ctx context.Context, q Querier, scan func(pgx.CollectableRow) (T, error), sql string, args ...any) ([]T, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scan)
}

func (r SQLReader) HasRelease(ctx context.Context, country, rel string, schemaVersion int) (bool, error) {
	rows, err := collect(ctx, r.Q, pgx.RowTo[int32], `SELECT 1 FROM release WHERE singleton AND country=$1 AND rel=$2 AND schema_version=$3`, country, rel, schemaVersion)
	if err != nil {
		return false, err
	}
	return len(rows) == 1, nil
}

func (r SQLReader) Documents(ctx context.Context, keys []string) ([]Document, error) {
	return collect(ctx, r.Q, func(row pgx.CollectableRow) (Document, error) {
		var d Document
		err := row.Scan(&d.ID, &d.Key, &d.Number, &d.NumberNorm, &d.TextSource, &d.TextComplete)
		return d, err
	}, `SELECT id,key,number,number_norm,text_source,text_complete FROM document WHERE key=ANY($1)`, keys)
}

func (r SQLReader) BaselineServed(ctx context.Context, keys []string) ([]string, error) {
	return collect(ctx, r.Q, pgx.RowTo[string], `SELECT item_key FROM coverage_gap WHERE stage='build' AND kind='baseline_served' AND item_key=ANY($1)`, keys)
}

func (r SQLReader) RelationRows(ctx context.Context, sourceDocumentIDs []int64) ([]RelationRow, error) {
	return collect(ctx, r.Q, func(row pgx.CollectableRow) (RelationRow, error) {
		var v RelationRow
		err := row.Scan(&v.Kind, &v.Method, &v.SourceDocumentID, &v.TargetDocumentID, &v.SourceProvisionID, &v.TargetProvisionID, &v.TargetNodeID)
		return v, err
	}, `SELECT kind,method,source_document_id,target_document_id,source_provision_id,target_provision_id,target_node_id FROM relation WHERE source_document_id=ANY($1) AND target_document_id IS NOT NULL`, sourceDocumentIDs)
}

func (r SQLReader) Provisions(ctx context.Context, documentID int64) ([]Provision, error) {
	return collect(ctx, r.Q, func(row pgx.CollectableRow) (Provision, error) {
		var p Provision
		err := row.Scan(&p.ID, &p.ParentID, &p.NodeID, &p.Anchor, &p.Heading)
		return p, err
	}, `SELECT id,parent_id,node_id,anchor,heading FROM provision WHERE document_id=$1 ORDER BY preorder`, documentID)
}

func (r SQLReader) ProvisionText(ctx context.Context, provisionID int64) (string, error) {
	texts, err := collect(ctx, r.Q, pgx.RowTo[string], `SELECT text FROM provision WHERE id=$1`, provisionID)
	if err != nil {
		return "", err
	}
	if len(texts) != 1 {
		return "", fmt.Errorf("provision %d not found", provisionID)
	}
	return texts[0], nil
}
