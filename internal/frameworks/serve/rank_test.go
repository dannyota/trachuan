package serve

import (
	"testing"

	"danny.vn/trachuan/internal/retrieval"
	"github.com/stretchr/testify/require"
)

func rankRecipe() recipe {
	r := recipe{Version: 2, Status: "placeholder", DenseDepth: 50, LexicalDepth: 50, RRF: retrieval.Recipe3().RRF}
	threshold := 0.5297041131124263
	r.Abstention.Statistic = "first-hit-best-permitted-cosine"
	r.Abstention.Threshold = &threshold
	r.Abstention.Status = "bootstrap-from-law"
	r.Source.Release = "vn/20261115_3"
	r.Source.RecipeVersion = 3
	r.Source.CalibrationSHA256 = "9264612fa2d933e3f0b1ee451e7707598192f9c66ed52224d48a9a327ca65113"
	return r
}

func TestRanking_OneGlobalFusionAndBestPermittedCosine(t *testing.T) {
	e := testEngine()
	e.recipe = rankRecipe()
	controls := map[int64]*control{
		10: {id: 10, versionID: 1, order: []int{1}},
		20: {id: 20, versionID: 2, order: []int{1}},
		30: {id: 30, versionID: 3, order: []int{1}},
	}
	chunks := []candidate{
		{chunkID: 1, controlID: 10, cosine: .8, bm25: 2, ordinal: 0},
		{chunkID: 2, controlID: 10, cosine: .9, bm25: 0, ordinal: 1},
		{chunkID: 3, controlID: 20, cosine: .9, bm25: 2},
		{chunkID: 4, controlID: 30, cosine: .9, bm25: 2},
	}
	ranked := e.rank(chunks, controls)
	require.Len(t, ranked, 3)
	require.Equal(t, int64(10), ranked[0].controlID)
	require.Equal(t, int64(20), ranked[1].controlID)
	require.Equal(t, int64(30), ranked[2].controlID)
	require.Equal(t, 1.5/61, ranked[0].score)
	require.Equal(t, 1.5/62, ranked[1].score)
	require.Equal(t, .9, ranked[0].cosine)
	require.Equal(t, int64(2), ranked[0].chunkID)
	e.recipe.DenseDepth = 1
	e.recipe.LexicalDepth = 1
	require.Len(t, e.rank(chunks, controls), 1)
}

func TestRanking_AbstentionUsesFirstFusedHit(t *testing.T) {
	e := testEngine()
	e.recipe = rankRecipe()
	controls := map[int64]*control{10: {id: 10, versionID: 1}, 20: {id: 20, versionID: 2}}
	chunks := []candidate{{chunkID: 1, controlID: 10, cosine: .3, bm25: 10}, {chunkID: 2, controlID: 20, cosine: .99, bm25: 0}}
	r := e.rank(chunks, controls)
	require.Equal(t, int64(10), r[0].controlID)
	response := Response{}
	e.abstention(&response, r)
	require.True(t, response.Abstain)
	require.Equal(t, "low_confidence", response.Reason)
	r[0].cosine = *e.recipe.Abstention.Threshold
	e.abstention(&response, r)
	require.False(t, response.Abstain)
	e.abstention(&response, nil)
	require.True(t, response.Abstain)
	require.Equal(t, "no_hits", response.Reason)
}

func TestRanking_TiesIgnoreDatabaseChunkIDs(t *testing.T) {
	e := testEngine()
	e.recipe = rankRecipe()
	controls := map[int64]*control{10: {id: 10, versionID: 1, order: []int{1, 2}}, 20: {id: 20, versionID: 1, order: []int{1, 1}}}
	for _, ids := range [][2]int64{{1, 999}, {1000, 1}} {
		chunks := []candidate{{chunkID: ids[0], controlID: 10, cosine: .8, bm25: 3}, {chunkID: ids[1], controlID: 20, cosine: .8, bm25: 3}}
		r := e.rank(chunks, controls)
		require.Equal(t, int64(20), r[0].controlID)
		require.Equal(t, int64(10), r[1].controlID)
	}
}

func TestHandle_CanonicalEscaping(t *testing.T) {
	require.Equal(t, "A.5%20%2F%20B(1)_~-", encodeCitation("A.5 / B(1)_~-"))
}
