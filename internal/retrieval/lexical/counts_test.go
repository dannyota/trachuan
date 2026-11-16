package lexical

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDocumentCounts_MatchesDocumentEncoding(t *testing.T) {
	r := Recipe1()
	stats := Stats{N: 3, TotalTokens: 8, AvgDL: 8.0 / 3, DF: map[string]int64{"a": 2, "b": 1}}
	want, err := Document("a a b", stats, r)
	require.NoError(t, err)
	got, err := DocumentCounts(map[string]int{"a": 2, "b": 1}, stats, r)
	require.NoError(t, err)
	require.Equal(t, want, got)
	_, err = DocumentCounts(map[string]int{"a": -1}, stats, r)
	require.Error(t, err)
	_, err = DocumentCounts(map[string]int{"a": 1}, stats, Recipe{})
	require.Error(t, err)
}
