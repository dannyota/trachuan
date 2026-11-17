package serve

import (
	"context"

	"danny.vn/trachuan/internal/retrieval"
	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/jackc/pgx/v5"
)

const bodyDenseSQL = `SELECT id,version_id,control_id,ordinal,token_count,1-(embedding <=> $2::vector)
 FROM body_chunk WHERE version_id=ANY($1::bigint[])`

const publicDenseSQL = `SELECT id,version_id,control_id,0,token_count,1-(embedding <=> $2::vector)
 FROM public_chunk WHERE version_id=ANY($1::bigint[])`

func (e *Engine) collect(ctx context.Context, tx pgx.Tx, ids []int64, p projection, vector []float32, query string, controls map[int64]*control) ([]candidate, error) {
	readable, reduced := []int64{}, []int64{}
	for _, id := range ids {
		if p.allowed[id] {
			readable = append(readable, id)
		} else {
			reduced = append(reduced, id)
		}
	}
	q, err := lexical.Query(query, e.lexical)
	if err != nil {
		return nil, ErrUnavailable
	}
	body, err := e.collectIndex(ctx, tx, readable, vector, q, false, controls)
	if err != nil {
		return nil, err
	}
	public, err := e.collectIndex(ctx, tx, reduced, vector, q, true, controls)
	if err != nil {
		return nil, err
	}
	return append(body, public...), nil
}

func (e *Engine) collectIndex(ctx context.Context, tx pgx.Tx, versions []int64, vector []float32, query lexical.Vector, public bool, controls map[int64]*control) ([]candidate, error) {
	if len(versions) == 0 {
		return nil, nil
	}
	sql := bodyDenseSQL
	if public {
		sql = publicDenseSQL
	}
	rows, err := tx.Query(ctx, sql, versions, retrieval.DenseVector(vector))
	if err != nil {
		return nil, ErrUnavailable
	}
	chunks := []candidate{}
	for rows.Next() {
		c := candidate{}
		if rows.Scan(&c.chunkID, &c.versionID, &c.controlID, &c.ordinal, &c.length, &c.cosine) != nil || !finite(c.cosine) || c.length <= 0 {
			rows.Close()
			return nil, ErrUnavailable
		}
		control, exists := controls[c.controlID]
		if !exists || control.versionID != c.versionID {
			rows.Close()
			return nil, ErrUnavailable
		}
		c.cosine = max(-1, min(1, c.cosine))
		chunks = append(chunks, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, ErrUnavailable
	}
	for i, c := range chunks {
		part := e.partitions[c.versionID]
		index := &part.public
		if !public {
			index = &part.body
		}
		chunk, ok := index.chunks[c.chunkID]
		if !ok || chunk.controlID != c.controlID || chunk.length != c.length || chunk.ordinal != c.ordinal {
			return nil, ErrUnavailable
		}
		chunks[i].bm25 = sparseDot(chunk.postings, query)
	}
	return chunks, nil
}

func sparseDot(document, query lexical.Vector) float64 {
	i, j := 0, 0
	var score float32
	for i < len(document) && j < len(query) {
		switch {
		case document[i].Index < query[j].Index:
			i++
		case query[j].Index < document[i].Index:
			j++
		default:
			score += document[i].Value * query[j].Value
			i++
			j++
		}
	}
	return float64(score)
}
