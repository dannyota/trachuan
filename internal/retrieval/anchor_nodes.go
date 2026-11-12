package retrieval

import (
	"context"
)

// AnchorRef identifies a provision in a document.
type AnchorRef struct {
	DocumentKey string
	Anchor      string
}

// AnchorNodes returns served effective node codes for known provisions.
func (e *Engine) AnchorNodes(ctx context.Context, refs []AnchorRef) (map[AnchorRef][]string, error) {
	result := map[AnchorRef][]string{}
	if e.release.SchemaVersion < 5 || len(refs) == 0 {
		return result, nil
	}
	ctx, cancel := context.WithTimeout(ctx, e.deadline())
	defer cancel()
	tx, err := e.metadataTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	keys, anchors := make([]string, 0, len(refs)), make([]string, 0, len(refs))
	for _, ref := range refs {
		keys = append(keys, ref.DocumentKey)
		anchors = append(anchors, ref.Anchor)
	}
	rows, err := tx.Query(ctx, `SELECT p.id,d.key,p.anchor FROM unnest($1::text[],$2::text[]) AS r(key,anchor) JOIN document d ON d.key=r.key JOIN provision p ON p.document_id=d.id AND p.anchor=r.anchor`, keys, anchors)
	if err != nil {
		return nil, databaseFailure("query anchor nodes", err)
	}
	ids := []int64{}
	byID := map[int64]AnchorRef{}
	for rows.Next() {
		var id int64
		var ref AnchorRef
		if err = rows.Scan(&id, &ref.DocumentKey, &ref.Anchor); err != nil {
			rows.Close()
			return nil, databaseFailure("scan anchor nodes", err)
		}
		ids = append(ids, id)
		byID[id] = ref
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, databaseFailure("read anchor nodes", err)
	}
	nodes, _, err := e.loadEffectiveNodes(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		codes := nodes[id]
		if codes == nil {
			codes = []string{}
		}
		result[byID[id]] = codes
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, databaseFailure("commit anchor nodes", err)
	}
	return result, nil
}
