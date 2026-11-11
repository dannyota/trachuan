package retrieval

import (
	"context"
	"errors"
	"maps"

	"github.com/jackc/pgx/v5"
)

// LookupDocument returns the original number and existence of a document key.
func (e *Engine) LookupDocument(ctx context.Context, key string) (Document, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, e.deadline())
	defer cancel()
	tx, err := e.metadataTx(ctx)
	if err != nil {
		return Document{}, false, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var doc Document
	err = tx.QueryRow(ctx, `SELECT number FROM document WHERE key=$1`, key).Scan(&doc.Number)
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, false, nil
	}
	if err != nil {
		return Document{}, false, databaseFailure("lookup report document", err)
	}
	return doc, true, nil
}

// AnchorExists checks an exact anchor within one document, including unchunked nodes.
func (e *Engine) AnchorExists(ctx context.Context, key, anchor string) (bool, error) {
	return e.reportExists(ctx, `SELECT EXISTS(SELECT 1 FROM provision p JOIN document d ON d.id=p.document_id WHERE d.key=$1 AND p.anchor=$2)`, key, anchor)
}

// HasNumber tests normalized-number membership without requiring uniqueness.
func (e *Engine) HasNumber(ctx context.Context, number string) (bool, error) {
	return e.reportExists(ctx, `SELECT EXISTS(SELECT 1 FROM document WHERE number_norm=$1)`, number)
}

func (e *Engine) reportExists(ctx context.Context, query string, args ...any) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, e.deadline())
	defer cancel()
	tx, err := e.metadataTx(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var exists bool
	if err = tx.QueryRow(ctx, query, args...).Scan(&exists); err != nil {
		return false, databaseFailure("lookup report identity", err)
	}
	return exists, nil
}

// NumberSegments returns a copy of the slash- and hyphen-separated number
// segments loaded once when this immutable release engine opened.
func (e *Engine) NumberSegments(ctx context.Context) (map[string]struct{}, error) {
	if err := ctx.Err(); err != nil {
		return nil, databaseFailure("read report number segments", err)
	}
	return maps.Clone(e.numberSegments), nil
}

func (e *Engine) loadNumberSegments(ctx context.Context) (map[string]struct{}, error) {
	ctx, cancel := context.WithTimeout(ctx, e.deadline())
	defer cancel()
	tx, err := e.metadataTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	rows, err := tx.Query(ctx, `SELECT DISTINCT segment FROM document CROSS JOIN LATERAL regexp_split_to_table(number_norm, '[/-]') AS segment WHERE segment <> ''`)
	if err != nil {
		return nil, databaseFailure("query report number segments", err)
	}
	defer rows.Close()
	segments := make(map[string]struct{})
	for rows.Next() {
		var segment string
		if err = rows.Scan(&segment); err != nil {
			return nil, databaseFailure("scan report number segment", err)
		}
		segments[segment] = struct{}{}
	}
	if err = rows.Err(); err != nil {
		return nil, databaseFailure("read report number segments", err)
	}
	return segments, nil
}
