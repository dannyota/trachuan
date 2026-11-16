package retrieval

import "danny.vn/trachuan/internal/embedding"

func DenseVector(values []float32) string { return denseVector(values) }

func ValidateDenseRecipe(recipe DenseRecipe, running embedding.Recipe, dimensions int) error {
	return validateDenseRecipe(recipe, running, dimensions)
}

func ReciprocalRank(k, rank int, weight float64) float64 {
	return weight / float64(k+rank)
}
