package serve

import (
	"context"

	"danny.vn/trachuan/internal/retrieval/lexical"
)

const bodyChunksSQL = `SELECT id,version_id,control_id,ordinal,token_count FROM body_chunk`
const publicChunksSQL = `SELECT id,version_id,control_id,0,token_count FROM public_chunk`
const bodyPostingsSQL = `SELECT version_id,chunk_id,token,tf FROM body_posting`
const publicPostingsSQL = `SELECT version_id,chunk_id,token,tf FROM public_posting`

func (e *Engine) loadIndex(ctx context.Context, public bool) error {
	indices := map[int64]*indexPartition{}
	for id, part := range e.partitions {
		index := &part.body
		if public {
			index = &part.public
		}
		*index = indexPartition{chunks: map[int64]indexChunk{}, stats: lexical.Stats{DF: map[string]int64{}}}
		indices[id] = index
	}
	if err := e.loadIndexChunks(ctx, indices, public); err != nil {
		return err
	}
	counts, temporaryBytes, err := e.loadIndexPostings(ctx, indices, public)
	if err != nil {
		return err
	}
	defer func() { e.partitionBytes -= temporaryBytes }()
	shared := lexical.Stats{DF: map[string]int64{}}
	for _, index := range indices {
		s := &index.stats
		s.AvgDL = 1
		if s.N > 0 {
			s.AvgDL = float64(s.TotalTokens) / float64(s.N)
		}
		if public {
			shared.N += s.N
			shared.TotalTokens += s.TotalTokens
			for token, df := range s.DF {
				if _, exists := shared.DF[token]; !exists {
					if err := e.reservePartitionMemory(int64(96 + len(token))); err != nil {
						return err
					}
				}
				shared.DF[token] += df
			}
		}
	}
	if public {
		shared.AvgDL = 1
		if shared.N > 0 {
			shared.AvgDL = float64(shared.TotalTokens) / float64(shared.N)
		}
	}
	for _, index := range indices {
		if public {
			index.stats = shared
		}
		for id, chunk := range index.chunks {
			if err := ctx.Err(); err != nil {
				return ErrUnavailable
			}
			if err := e.reservePartitionMemory(int64(16 * len(counts[id]))); err != nil {
				return err
			}
			document, encodeErr := encodePostings(counts[id], chunk.length, index.stats, e.lexical)
			if encodeErr != nil {
				return encodeErr
			}
			chunk.postings = document
			index.chunks[id] = chunk
		}
	}
	return nil
}

func (e *Engine) loadIndexChunks(ctx context.Context, indices map[int64]*indexPartition, public bool) error {
	sql := bodyChunksSQL
	if public {
		sql = publicChunksSQL
	}
	rows, err := e.db.Query(ctx, sql)
	if err != nil {
		return ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var id, versionID int64
		var chunk indexChunk
		if rows.Scan(&id, &versionID, &chunk.controlID, &chunk.ordinal, &chunk.length) != nil || chunk.length <= 0 {
			return ErrUnavailable
		}
		index := indices[versionID]
		if index == nil || e.partitions[versionID].controls[chunk.controlID] == nil {
			return ErrUnavailable
		}
		if err := e.reservePartitionMemory(128); err != nil {
			return err
		}
		index.chunks[id] = chunk
		index.stats.N++
		index.stats.TotalTokens += int64(chunk.length)
	}
	if rows.Err() != nil {
		return ErrUnavailable
	}
	return nil
}

func (e *Engine) loadIndexPostings(ctx context.Context, indices map[int64]*indexPartition, public bool) (map[int64]map[string]int, int64, error) {
	sql := bodyPostingsSQL
	if public {
		sql = publicPostingsSQL
	}
	rows, err := e.db.Query(ctx, sql)
	if err != nil {
		return nil, 0, ErrUnavailable
	}
	defer rows.Close()
	counts := map[int64]map[string]int{}
	var temporaryBytes int64
	for rows.Next() {
		var versionID, id int64
		var token string
		var tf int
		if rows.Scan(&versionID, &id, &token, &tf) != nil || tf <= 0 {
			return nil, 0, ErrUnavailable
		}
		index := indices[versionID]
		if index == nil {
			return nil, 0, ErrUnavailable
		}
		if _, ok := index.chunks[id]; !ok {
			return nil, 0, ErrUnavailable
		}
		bytes := int64(96 + len(token))
		if err := e.reservePartitionMemory(bytes); err != nil {
			return nil, 0, err
		}
		temporaryBytes += bytes
		if _, exists := index.stats.DF[token]; !exists {
			if err := e.reservePartitionMemory(int64(96 + len(token))); err != nil {
				return nil, 0, err
			}
		}
		if counts[id] == nil {
			counts[id] = map[string]int{}
		}
		counts[id][token] = tf
		index.stats.DF[token]++
	}
	if rows.Err() != nil {
		return nil, 0, ErrUnavailable
	}
	return counts, temporaryBytes, nil
}

func encodePostings(counts map[string]int, length int, stats lexical.Stats, recipe lexical.Recipe) (lexical.Vector, error) {
	total := 0
	for _, tf := range counts {
		total += tf
	}
	if total != length {
		return nil, ErrUnavailable
	}
	document, err := lexical.DocumentCounts(counts, stats, recipe)
	if err != nil {
		return nil, ErrUnavailable
	}
	return document, nil
}
