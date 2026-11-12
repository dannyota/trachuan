package retrieval

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// Status contains the document portion of a hit's validity stamp.
type Status struct {
	Class       string `json:"class,omitempty"`
	Basis       string `json:"basis,omitempty"`
	StatusBasis string `json:"status_basis,omitempty"`
	Current     bool   `json:"current"`
}

// DocumentEntry contains document metadata and bounded relation pointers.
type DocumentEntry struct {
	Number        string         `json:"number,omitempty"`
	Title         string         `json:"title,omitempty"`
	Type          string         `json:"type,omitempty"`
	Issuer        string         `json:"issuer,omitempty"`
	Issued        *Date          `json:"issued,omitempty"`
	EffectiveFrom *Date          `json:"effective_from,omitempty"`
	EffectiveTo   *Date          `json:"effective_to,omitempty"`
	Status        Status         `json:"status"`
	TextSource    string         `json:"text_source"`
	TextComplete  bool           `json:"text_complete"`
	SourceURL     string         `json:"source_url,omitempty"`
	Related       []Pointer      `json:"related,omitempty"`
	RelatedCounts map[string]int `json:"related_counts,omitempty"`
	Nodes         []string       `json:"nodes,omitempty"`
}

func (e *Engine) loadDocuments(ctx context.Context, keys []string) (map[string]DocumentEntry, error) {
	if len(keys) == 0 {
		return map[string]DocumentEntry{}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, e.deadline())
	defer cancel()
	tx, err := e.db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, databaseFailure("begin documents transaction", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err = setLocal(ctx, tx, "statement_timeout", strconv.FormatInt(e.deadline().Milliseconds(), 10)); err != nil {
		return nil, fmt.Errorf("set documents deadline: %w", err)
	}
	docs := make(map[string]DocumentEntry)
	rows, err := tx.Query(ctx, `SELECT key, number, title, doc_type, issuer, issue_date, eff_from, eff_to, COALESCE(source_url, ''), text_source, text_complete FROM document WHERE key = ANY($1)`, keys)
	if err != nil {
		return nil, databaseFailure("query document metadata", err)
	}
	for rows.Next() {
		var key string
		var doc DocumentEntry
		var issued, from, to *time.Time
		if err = rows.Scan(&key, &doc.Number, &doc.Title, &doc.Type, &doc.Issuer, &issued, &from, &to, &doc.SourceURL, &doc.TextSource, &doc.TextComplete); err != nil {
			rows.Close()
			return nil, databaseFailure("scan document metadata", err)
		}
		doc.Issued, doc.EffectiveFrom, doc.EffectiveTo = dateValue(issued), dateValue(from), dateValue(to)
		docs[key] = doc
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, databaseFailure("read document metadata", err)
	}
	rows, err = tx.Query(ctx, documentCountsSQL, keys)
	if err != nil {
		return nil, databaseFailure("query document relation counts", err)
	}
	for rows.Next() {
		var key, dir, kind string
		var count int
		if err = rows.Scan(&key, &dir, &kind, &count); err != nil {
			rows.Close()
			return nil, databaseFailure("scan document relation counts", err)
		}
		doc := docs[key]
		if doc.RelatedCounts == nil {
			doc.RelatedCounts = make(map[string]int)
		}
		doc.RelatedCounts[dir+":"+kind] = count
		docs[key] = doc
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, databaseFailure("read document relation counts", err)
	}
	rows, err = tx.Query(ctx, documentPointersSQL, keys, pointerKinds(), PointersPerDocument)
	if err != nil {
		return nil, databaseFailure("query document pointers", err)
	}
	for rows.Next() {
		var key string
		var pointer Pointer
		if err = rows.Scan(&key, &pointer.Dir, &pointer.Kind, &pointer.Key, &pointer.Number, &pointer.InRelease); err != nil {
			rows.Close()
			return nil, databaseFailure("scan document pointers", err)
		}
		doc := docs[key]
		doc.Related = append(doc.Related, pointer)
		docs[key] = doc
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, databaseFailure("read document pointers", err)
	}
	nodes, err := e.loadDocumentNodes(ctx, tx, keys)
	if err != nil {
		return nil, err
	}
	for key, doc := range docs {
		doc.Nodes = append([]string(nil), nodes[key]...)
		docs[key] = doc
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, databaseFailure("commit documents transaction", err)
	}
	return docs, nil
}
