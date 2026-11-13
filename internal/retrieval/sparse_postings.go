package retrieval

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"

	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type sparsePosting struct {
	position uint32
	weight   float32
}
type sparseDimension struct {
	index      int32
	start, end uint32
}
type sparseChunk struct {
	id       int64
	document uint32
}
type sparseDocument struct {
	current, conflict     bool
	from, to              pgtype.Date
	class, number, status string
}
type sparsePostings struct {
	dimensions []sparseDimension
	entries    []sparsePosting
	chunks     []sparseChunk
	documents  []sparseDocument
	delta      float64
	pool       sync.Pool
}
type sparseScores struct {
	values  []float32
	touched []uint32
}

func (d sparseDocument) eligible(date time.Time, historical, notYet bool) bool {
	from := !d.from.Valid || d.from.InfinityModifier == pgtype.NegativeInfinity || (d.from.InfinityModifier == pgtype.Finite && !d.from.Time.After(date))
	to := !d.to.Valid || d.to.InfinityModifier == pgtype.Infinity || (d.to.InfinityModifier == pgtype.Finite && date.Before(d.to.Time))
	return ((historical || (d.current && !d.conflict)) && from && to) || (notYet && !historical && d.status == "not_yet")
}

type sparseScored struct {
	id    int64
	score float64
}

func sparseCandidateOrder(a, b sparseScored) int {
	if n := cmp.Compare(b.score, a.score); n != 0 {
		return n
	}
	return cmp.Compare(a.id, b.id)
}

func sparseCandidates(candidates []sparseScored, depth int, delta float64) []sparseScored {
	slices.SortFunc(candidates, sparseCandidateOrder)
	if len(candidates) <= depth {
		return candidates
	}
	cutoff := candidates[depth-1].score - 2*delta
	end := depth
	for end < len(candidates) && candidates[end].score >= cutoff {
		end++
	}
	return candidates[:end]
}

func (p *sparsePostings) candidates(ctx context.Context, query lexical.Vector, date time.Time, historical, notYet bool, refs []string, depth int) ([2][]sparseScored, error) {
	var groups [2][]sparseScored
	scores, _ := p.pool.Get().(*sparseScores)
	if scores == nil {
		scores = &sparseScores{values: make([]float32, len(p.chunks))}
	}
	defer func() {
		for _, pos := range scores.touched {
			scores.values[pos] = 0
		}
		scores.touched = scores.touched[:0]
		p.pool.Put(scores)
	}()
	for _, term := range query {
		if err := ctx.Err(); err != nil {
			return groups, err
		}
		i, found := slices.BinarySearchFunc(p.dimensions, term.Index, func(d sparseDimension, index int32) int { return cmp.Compare(d.index, index) })
		if !found {
			continue
		}
		d := p.dimensions[i]
		for i, entry := range p.entries[d.start:d.end] {
			if i%4096 == 0 {
				if err := ctx.Err(); err != nil {
					return groups, err
				}
			}
			if scores.values[entry.position] == 0 {
				scores.touched = append(scores.touched, entry.position)
			}
			scores.values[entry.position] += entry.weight
		}
	}
	for i, pos := range scores.touched {
		if i%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return groups, err
			}
		}
		chunk := p.chunks[pos]
		doc := p.documents[chunk.document]
		if doc.class == "relation_context" && !slices.Contains(refs, doc.number) {
			continue
		}
		group := 1
		if doc.eligible(date, historical, notYet) {
			group = 0
		}
		groups[group] = append(groups[group], sparseScored{id: chunk.id, score: float64(scores.values[pos])})
	}
	for i := range groups {
		groups[i] = sparseCandidates(groups[i], depth, p.delta)
	}
	return groups, nil
}

const sparseRescoreSQL = `SELECT id, -(sparse <#> $1::sparsevec) FROM chunk WHERE id = ANY($2::bigint[]) ORDER BY 2 DESC, id`

func (p *sparsePostings) lexical(ctx context.Context, tx pgx.Tx, query lexical.Vector, sv string, date time.Time, historical, notYet bool, refs []string, depth int) ([2][]Candidate, error) {
	var result [2][]Candidate
	candidates, err := p.candidates(ctx, query, date, historical, notYet, refs, depth)
	if err != nil {
		return result, err
	}
	ids := make([]int64, 0, len(candidates[0])+len(candidates[1]))
	groupByID := make(map[int64]int, cap(ids))
	for i, group := range candidates {
		for _, c := range group {
			ids = append(ids, c.id)
			groupByID[c.id] = i
		}
	}
	rows, err := tx.Query(ctx, sparseRescoreSQL, sv, ids)
	if err != nil {
		return result, databaseFailure("rescore sparse candidates failed", err)
	}
	defer rows.Close()
	for rows.Next() {
		var c Candidate
		if err = rows.Scan(&c.ChunkID, &c.BM25); err != nil {
			return result, databaseFailure("read sparse candidate failed", err)
		}
		if c.BM25 > 0 {
			i := groupByID[c.ChunkID]
			if len(result[i]) < depth {
				c.Rank = len(result[i]) + 1
				result[i] = append(result[i], c)
			}
		}
	}
	if err = rows.Err(); err != nil {
		return result, databaseFailure("read sparse candidates failed", err)
	}
	return result, nil
}
