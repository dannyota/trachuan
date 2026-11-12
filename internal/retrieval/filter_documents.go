package retrieval

import (
	"context"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"golang.org/x/text/unicode/norm"
)

type filterDocumentReader interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func (e *Engine) filterDocuments(ctx context.Context, keys []string) (map[string]DocumentEntry, error) {
	if len(keys) == 0 {
		return map[string]DocumentEntry{}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, e.deadline())
	defer cancel()
	tx, err := e.metadataTx(ctx)
	if err != nil {
		return nil, err
	}
	defer rollbackReport(ctx, tx, reportRollbackTimeout)
	docs, err := queryFilterDocuments(ctx, tx, keys)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, databaseFailure("commit filter metadata", err)
	}
	return docs, nil
}

func queryFilterDocuments(ctx context.Context, db filterDocumentReader, keys []string) (map[string]DocumentEntry, error) {
	docs := map[string]DocumentEntry{}
	if len(keys) == 0 {
		return docs, nil
	}
	keys = slices.Clone(keys)
	slices.Sort(keys)
	keys = slices.Compact(keys)
	// Filtering needs no relations, node counts or other document metadata.
	rows, err := db.Query(ctx, `SELECT key,doc_type,issuer FROM document WHERE key=ANY($1::text[])`, keys)
	if err != nil {
		return nil, databaseFailure("query filter metadata", err)
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var doc DocumentEntry
		if err = rows.Scan(&key, &doc.Type, &doc.Issuer); err != nil {
			return nil, databaseFailure("scan filter metadata", err)
		}
		doc.Type = norm.NFC.String(strings.TrimSpace(doc.Type))
		doc.Issuer = norm.NFC.String(strings.TrimSpace(doc.Issuer))
		docs[key] = doc
	}
	if err = rows.Err(); err != nil {
		return nil, databaseFailure("read filter metadata", err)
	}
	return docs, nil
}
