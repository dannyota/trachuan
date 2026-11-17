package serve

import (
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func filterRankFixture() (*Engine, []candidate, map[int64]*control) {
	e := testEngine()
	e.recipe = rankRecipe()
	e.recipe.DenseDepth, e.recipe.LexicalDepth = 9, 8
	controls := map[int64]*control{}
	chunks := []candidate{}
	for id := int64(1); id <= 10; id++ {
		versionID := int64(1)
		switch id {
		case 8:
			versionID = 2
		case 10:
			versionID = 3
		}
		controls[id] = &control{id: id, versionID: versionID, order: []int{int(id)}}
		c := candidate{chunkID: id, controlID: id, versionID: versionID, cosine: 1 - float64(id)/100, bm25: float64(10 - id)}
		switch id {
		case 8:
			c.cosine, c.bm25 = .91, 1
		case 9:
			c.cosine, c.bm25 = .92, 0
		case 10:
			c.cosine, c.bm25 = .1, 2
		}
		chunks = append(chunks, c)
	}
	return e, chunks, controls
}

func allowedRankControls(e *Engine, controls map[int64]*control, filters map[string][]string) map[int64]*control {
	allowed := map[int64]*control{}
	for _, id := range e.selectVersions(Request{Filters: filters}) {
		for key, c := range controls {
			if c.versionID == id {
				allowed[key] = c
			}
		}
	}
	return allowed
}

func TestRanking_FilterKeepsEighthGlobalHit(t *testing.T) {
	e, chunks, controls := filterRankFixture()
	open := e.rank(chunks, controls)
	require.Equal(t, int64(9), open[7].controlID)
	for _, filters := range []map[string][]string{
		{"frameworks": {"fw-alpha"}},
		{"versions": {"fw:fw-alpha:1", "fw:fw-alpha:2"}},
	} {
		allowed := allowedRankControls(e, controls, filters)
		for _, hit := range open[:8] {
			require.Contains(t, allowed, hit.controlID)
		}
		filtered := e.rankFiltered(chunks, controls, allowed)
		require.Equal(t, open[:8], filtered[:8])
	}
}

func TestRanking_FiltersNeverWorsenGlobalPositions(t *testing.T) {
	e, fixture, controls := filterRankFixture()
	for query := 0; query < 16; query++ {
		t.Run(fmt.Sprintf("invented-query-%d", query), func(t *testing.T) {
			chunks := slices.Clone(fixture)
			if query > 0 {
				for i := range chunks {
					chunks[i].cosine = float64((i*7+query*3)%19)/10 - 1
					chunks[i].bm25 = float64((i*3 + query*7) % 13)
				}
			}
			for _, depth := range []int{1, 3, 9, 50} {
				e.recipe.DenseDepth, e.recipe.LexicalDepth = depth, min(depth, 8)
				open := e.rank(chunks, controls)
				for mask := 0; mask < 8; mask++ {
					allowed := map[int64]*control{}
					for id, c := range controls {
						if mask&(1<<uint(c.versionID-1)) != 0 {
							allowed[id] = c
						}
					}
					filtered := e.rankFiltered(chunks, controls, allowed)
					positions := map[int64]int{}
					for i, hit := range filtered {
						require.Contains(t, allowed, hit.controlID)
						require.NotContains(t, positions, hit.controlID)
						positions[hit.controlID] = i
					}
					for i, hit := range open {
						if _, ok := allowed[hit.controlID]; ok {
							require.Contains(t, positions, hit.controlID)
							require.LessOrEqual(t, positions[hit.controlID], i)
							require.Equal(t, hit, filtered[positions[hit.controlID]])
						}
					}
				}
			}
		})
	}
}

func TestRanking_FilterFillKeepsOrderScoresAndChunks(t *testing.T) {
	e := testEngine()
	e.recipe = rankRecipe()
	e.recipe.DenseDepth, e.recipe.LexicalDepth = 3, 3
	controls := map[int64]*control{
		1: {id: 1, versionID: 1}, 2: {id: 2, versionID: 1},
		3: {id: 3, versionID: 1}, 4: {id: 4, versionID: 1},
		5: {id: 5, versionID: 3}, 6: {id: 6, versionID: 3},
	}
	chunks := []candidate{
		{controlID: 1, chunkID: 11, cosine: .8},
		{controlID: 2, chunkID: 21, cosine: .7},
		{controlID: 2, chunkID: 22, cosine: .65, bm25: 8, ordinal: 1},
		{controlID: 3, chunkID: 31, cosine: .6, bm25: 7},
		{controlID: 4, chunkID: 41, cosine: .5, bm25: 6},
		{controlID: 5, chunkID: 51, cosine: .95, bm25: 10},
		{controlID: 6, chunkID: 61, cosine: .9, bm25: 9},
	}
	allowed := allowedRankControls(e, controls, map[string][]string{"frameworks": {"fw-alpha"}})
	got := e.rankFiltered(chunks, controls, allowed)
	require.Equal(t, []ranked{
		{controlID: 1, chunkID: 11, score: 1.0 / 63, cosine: .8},
		{controlID: 2, chunkID: 22, score: .5 / 63, cosine: .7, ordinal: 1},
		{controlID: 3, chunkID: 31, score: 1.0/63 + .5/62, cosine: .6},
		{controlID: 4, chunkID: 41, score: .5 / 63, cosine: .5},
	}, got)
	require.Equal(t, []ranked{{controlID: 4, chunkID: 41, score: 1.5 / 61, cosine: .5}},
		e.rankFiltered(chunks, controls, map[int64]*control{4: controls[4]}))
}

func TestRanking_FilterAbstentionUsesFirstListedCosine(t *testing.T) {
	e := testEngine()
	e.recipe = rankRecipe()
	controls := map[int64]*control{1: {id: 1, versionID: 1}, 2: {id: 2, versionID: 3}}
	chunks := []candidate{{controlID: 1, chunkID: 11, cosine: .1}, {controlID: 2, chunkID: 21, cosine: .99, bm25: 1}}
	allowed := allowedRankControls(e, controls, map[string][]string{"versions": {"fw:fw-alpha:2"}})
	for _, depth := range []int{1, 2} {
		e.recipe.DenseDepth, e.recipe.LexicalDepth = depth, depth
		response := Response{}
		e.abstention(&response, e.rankFiltered(chunks, controls, allowed))
		require.True(t, response.Abstain)
		require.Equal(t, "low_confidence", response.Reason)
	}
	response := Response{}
	e.abstention(&response, e.rankFiltered(chunks, controls, nil))
	require.True(t, response.Abstain)
	require.Equal(t, "no_hits", response.Reason)
}

func TestRanking_FilterAbstainsBeforeHigherCosineFill(t *testing.T) {
	e := testEngine()
	e.recipe = rankRecipe()
	e.recipe.DenseDepth, e.recipe.LexicalDepth = 1, 1
	controls := map[int64]*control{
		1: {id: 1, versionID: 1}, 2: {id: 2, versionID: 1}, 3: {id: 3, versionID: 3},
	}
	chunks := []candidate{
		{controlID: 1, chunkID: 11, cosine: .1, bm25: 10},
		{controlID: 2, chunkID: 21, cosine: .9},
		{controlID: 3, chunkID: 31, cosine: .99},
	}
	allowed := allowedRankControls(e, controls, map[string][]string{"frameworks": {"fw-alpha"}})
	hits := e.rankFiltered(chunks, controls, allowed)
	require.Len(t, hits, 2)
	require.Equal(t, int64(1), hits[0].controlID)
	require.Equal(t, int64(2), hits[1].controlID)
	response := Response{}
	e.abstention(&response, hits)
	require.True(t, response.Abstain)
	require.Equal(t, "low_confidence", response.Reason)
}
