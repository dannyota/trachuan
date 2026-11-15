package build

import (
	"context"
	"fmt"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"github.com/jackc/pgx/v5/pgxpool"
)

func CheckCorrections(ctx context.Context, db *pgxpool.Pool, entries []source.Correction) ([]error, error) {
	keys := make([]string, 0, len(entries)*2)
	for _, entry := range entries {
		keys = append(keys, entry.Key)
		if entry.Evidence.Key != "" {
			keys = append(keys, entry.Evidence.Key)
		}
	}
	docs := map[string]correctionDocument{}
	rows, err := db.Query(ctx, `SELECT key, number, eff_from, eff_to FROM document WHERE key = ANY($1)`, keys)
	if err != nil {
		return nil, fmt.Errorf("read correction documents: %w", err)
	}
	for rows.Next() {
		var key, number string
		var from, to *time.Time
		if err = rows.Scan(&key, &number, &from, &to); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan correction document: %w", err)
		}
		docs[key] = correctionDocument{number: number, from: from, to: to, provisions: map[string]string{}}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("read correction documents: %w", err)
	}
	rows, err = db.Query(ctx, `SELECT d.key, p.id, p.parent_id, p.label, p.anchor, p.text FROM provision p JOIN document d ON d.id=p.document_id WHERE d.key = ANY($1) ORDER BY d.key, p.preorder`, keys)
	if err != nil {
		return nil, fmt.Errorf("read correction provisions: %w", err)
	}
	labels := map[int64]string{}
	for rows.Next() {
		var key, label, anchor, text string
		var id int64
		var parent *int64
		if err = rows.Scan(&key, &id, &parent, &label, &anchor, &text); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan correction provision: %w", err)
		}
		addCorrectionEvidence(docs, labels, key, id, parent, label, anchor, text)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("read correction provisions: %w", err)
	}
	return checkCorrections(entries, docs), nil
}
