package serve

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecipe_MeasuredRequiresCalibrationHash(t *testing.T) {
	r := rankRecipe()
	r.Status, r.Abstention.Status = "measured", "measured"
	r.Abstention.CalibrationSHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	require.NoError(t, r.validate())
	r.Abstention.CalibrationSHA256 = ""
	require.ErrorIs(t, r.validate(), ErrUnavailable)
}

func TestCalibrationRecipeHash_ExcludesFittedThreshold(t *testing.T) {
	e := testEngine()
	e.recipe = rankRecipe()
	before := e.CalibrationRecipeHash()
	require.Len(t, before, 64)
	value := .9
	e.recipe.Abstention.Threshold = &value
	e.recipe.Status, e.recipe.Abstention.Status = "measured", "measured"
	e.recipe.Abstention.CalibrationSHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	require.Equal(t, before, e.CalibrationRecipeHash())
	e.recipe.DenseDepth++
	require.NotEqual(t, before, e.CalibrationRecipeHash())
}
