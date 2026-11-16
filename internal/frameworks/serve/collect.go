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
	indices := map[int64]int{}
	stats := map[int64]lexical.Stats{}
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
		indices[c.chunkID] = len(chunks)
		chunks = append(chunks, c)
		s := stats[c.versionID]
		s.N++
		s.TotalTokens += int64(c.length)
		if s.DF == nil {
			s.DF = map[string]int64{}
		}
		stats[c.versionID] = s
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, ErrUnavailable
	}
	postings, err := readPostings(ctx, tx, versions, indices, public)
	if err != nil {
		return nil, err
	}
	for id, counts := range postings {
		c := chunks[indices[id]]
		s := stats[c.versionID]
		for token := range counts {
			s.DF[token]++
		}
	}
	for id, s := range stats {
		s.AvgDL = float64(s.TotalTokens) / float64(s.N)
		stats[id] = s
	}
	var publicStats lexical.Stats
	if public {
		publicStats, err = readPublicStats(ctx, tx)
		if err != nil {
			return nil, err
		}
	}
	for i, c := range chunks {
		s := stats[c.versionID]
		if public {
			s = publicStats
		}
		counts := postings[c.chunkID]
		length := 0
		for _, count := range counts {
			length += count
		}
		if length != c.length {
			return nil, ErrUnavailable
		}
		document, encodeErr := lexical.DocumentCounts(counts, s, e.lexical)
		if encodeErr != nil {
			return nil, ErrUnavailable
		}
		chunks[i].bm25 = sparseDot(document, query)
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
