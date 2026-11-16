package serve

import (
	"slices"
	"sort"

	"danny.vn/trachuan/internal/retrieval"
)

type candidate struct {
	chunkID, controlID, versionID int64
	ordinal, length               int
	cosine, bm25                  float64
}

type ranked struct {
	controlID, chunkID int64
	score, cosine      float64
	ordinal            int
}

func (e *Engine) less(a, b *control) bool {
	if a.versionID != b.versionID {
		for _, v := range e.versions {
			if v.id == a.versionID {
				return true
			}
			if v.id == b.versionID {
				return false
			}
		}
	}
	if compared := slices.Compare(a.order, b.order); compared != 0 {
		return compared < 0
	}
	return a.citation < b.citation
}

func (e *Engine) rank(chunks []candidate, controls map[int64]*control) []ranked {
	byControl := map[int64][]candidate{}
	for _, c := range chunks {
		byControl[c.controlID] = append(byControl[c.controlID], c)
	}
	dense, lex := []candidate{}, []candidate{}
	for _, cs := range byControl {
		d, l := cs[0], cs[0]
		for _, c := range cs[1:] {
			if c.cosine > d.cosine || (c.cosine == d.cosine && c.ordinal < d.ordinal) {
				d = c
			}
			if c.bm25 > l.bm25 || (c.bm25 == l.bm25 && c.ordinal < l.ordinal) {
				l = c
			}
		}
		dense = append(dense, d)
		if l.bm25 > 0 {
			lex = append(lex, l)
		}
	}
	sort.Slice(dense, func(i, j int) bool {
		if dense[i].cosine != dense[j].cosine {
			return dense[i].cosine > dense[j].cosine
		}
		return e.less(controls[dense[i].controlID], controls[dense[j].controlID])
	})
	sort.Slice(lex, func(i, j int) bool {
		if lex[i].bm25 != lex[j].bm25 {
			return lex[i].bm25 > lex[j].bm25
		}
		return e.less(controls[lex[i].controlID], controls[lex[j].controlID])
	})
	fused := map[int64]*ranked{}
	chunkScores := map[int64]map[int64]float64{}
	add := func(arm []candidate, depth int, weight float64) {
		for i, c := range arm[:min(depth, len(arm))] {
			r, ok := fused[c.controlID]
			if !ok {
				r = &ranked{controlID: c.controlID}
				fused[c.controlID] = r
				chunkScores[c.controlID] = map[int64]float64{}
			}
			score := retrieval.ReciprocalRank(e.recipe.RRF.K, i+1, weight)
			r.score += score
			chunkScores[c.controlID][c.chunkID] += score
		}
	}
	add(dense, e.recipe.DenseDepth, e.recipe.RRF.DenseWeight)
	add(lex, e.recipe.LexicalDepth, e.recipe.RRF.LexicalWeight)
	result := make([]ranked, 0, len(fused))
	for id, r := range fused {
		r.cosine = -1
		best := -1.0
		for _, c := range byControl[id] {
			r.cosine = max(r.cosine, c.cosine)
			score := chunkScores[id][c.chunkID]
			if score > best || (score == best && c.ordinal < r.ordinal) {
				best, r.chunkID, r.ordinal = score, c.chunkID, c.ordinal
			}
		}
		result = append(result, *r)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].score != result[j].score {
			return result[i].score > result[j].score
		}
		return e.less(controls[result[i].controlID], controls[result[j].controlID])
	})
	return result
}

func (e *Engine) abstention(response *Response, hits []ranked) {
	response.Abstain, response.Reason, response.Message = false, "", ""
	if len(hits) == 0 {
		response.Abstain, response.Reason = true, "no_hits"
	} else if hits[0].cosine < *e.recipe.Abstention.Threshold {
		response.Abstain, response.Reason = true, "low_confidence"
	}
}
