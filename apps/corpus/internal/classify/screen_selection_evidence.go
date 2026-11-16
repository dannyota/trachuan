package classify

import (
	"context"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
	"github.com/jackc/pgx/v5/pgxpool"
)

func LoadSelectedDocuments(ctx context.Context, release, workshop *pgxpool.Pool, artifacts artifact.Store, labels taxonomy.Labels) ([]Document, error) {
	nodes, err := LoadNodes(ctx, release)
	if err != nil {
		return nil, err
	}
	articles, err := LoadArticles(ctx, release, labels.Articles)
	if err != nil {
		return nil, err
	}
	if err = ValidateScreenSelection(labels, articles, nodes); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(labels.Articles))
	for _, label := range labels.Articles {
		keys = append(keys, label.Key)
	}
	rows, err := release.Query(ctx, `SELECT key,title,fetch_sha256 FROM document WHERE class='primary' AND key=ANY($1) ORDER BY key`, keys)
	if err != nil {
		return nil, fmt.Errorf("read selected classification documents: %w", err)
	}
	var primary []Document
	hashes := map[string]string{}
	for rows.Next() {
		var d Document
		var hash string
		if err = rows.Scan(&d.Key, &d.Title, &hash); err != nil {
			rows.Close()
			return nil, fmt.Errorf("decode selected classification document: %w", err)
		}
		primary = append(primary, d)
		hashes[d.Key] = hash
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("read selected classification documents: %w", err)
	}
	selected, err := SelectLabelDocuments(labels.Articles, primary)
	if err != nil {
		return nil, err
	}
	return loadDocumentEvidence(ctx, release, workshop, artifacts, selected, hashes)
}
