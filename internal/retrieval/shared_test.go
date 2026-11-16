package retrieval

import (
	"testing"

	"danny.vn/trachuan/internal/embedding"
	"github.com/stretchr/testify/require"
)

func TestShared_DensePrimitives(t *testing.T) {
	v := []float32{0.25, -0.5, 1}
	require.Equal(t, denseVector(v), DenseVector(v))
	doc, query := embedding.DocumentRecipe1(), embedding.QueryRecipe1()
	r := DenseRecipe{1, doc.Model, doc.Dimensions, doc, doc.Hash(), query, query.Hash()}
	require.NoError(t, ValidateDenseRecipe(r, query, doc.Dimensions))
	require.Error(t, ValidateDenseRecipe(r, query, doc.Dimensions+1))
}

func TestReciprocalRank_WeightsAndOneBasedRank(t *testing.T) {
	require.Equal(t, 1.0/61, ReciprocalRank(60, 1, 1))
	require.Equal(t, 0.5/62, ReciprocalRank(60, 2, 0.5))
}
