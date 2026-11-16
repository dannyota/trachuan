package serve

import (
	"testing"

	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/stretchr/testify/require"
)

func TestSparseDot_MatchesLawFloat32Accumulation(t *testing.T) {
	document := lexical.Vector{{Index: 1, Value: 16777216}, {Index: 2, Value: 1}, {Index: 3, Value: 1}}
	query := lexical.Vector{{Index: 1, Value: 1}, {Index: 2, Value: 1}, {Index: 3, Value: 1}}
	require.Equal(t, float64(16777216), sparseDot(document, query))
	require.Equal(t, float64(1), sparseDot(document, lexical.Vector{{Index: 3, Value: 1}, {Index: 4, Value: 1}}))
	require.Zero(t, sparseDot(document, nil))
}
