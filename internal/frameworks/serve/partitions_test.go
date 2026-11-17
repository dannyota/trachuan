package serve

import (
	"testing"

	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/stretchr/testify/require"
)

func TestPartitionPostings_PreserveScoresAndRejectIncompleteLengths(t *testing.T) {
	r := lexical.Recipe1()
	stats := lexical.Stats{N: 2, AvgDL: 3, DF: map[string]int64{"quartz": 2, "safeguard": 1}}
	postings, err := encodePostings(map[string]int{"quartz": 2, "safeguard": 1}, 3, stats, r)
	require.NoError(t, err)
	query, err := lexical.Query("quartz safeguard", r)
	require.NoError(t, err)
	document, err := lexical.Document("quartz quartz safeguard", stats, r)
	require.NoError(t, err)
	require.Equal(t, sparseDot(document, query), sparseDot(postings, query))
	for _, length := range []int{2, 4} {
		_, err = encodePostings(map[string]int{"quartz": 2, "safeguard": 1}, length, stats, r)
		require.ErrorIs(t, err, ErrUnavailable)
	}
	_, err = encodePostings(map[string]int{"missing": 1}, 1, stats, r)
	require.ErrorIs(t, err, ErrUnavailable)
}

func TestCachedMappingEnd_UsesOnlyResolvedPublicFields(t *testing.T) {
	e := testEngine()
	e.partitions = map[int64]*partition{1: {citations: map[string]*control{"A-1": {title: "Invented safeguard"}}}}
	key := []string{"fw-alpha", "2", "A-1"}
	id := int64(1)
	end := e.cachedMappingEnd(key, &id)
	require.Equal(t, "fw:fw-alpha:2#A-1", end.Handle)
	require.Equal(t, "Invented safeguard", end.Title)
	unresolved := e.cachedMappingEnd(key, nil)
	require.Empty(t, unresolved.Handle)
	require.Empty(t, unresolved.Title)
	require.Equal(t, "fw-alpha", unresolved.Code)
}
