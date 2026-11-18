package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type recipeVector struct {
	Name       string          `json:"name"`
	SHA256     string          `json:"sha256"`
	Normalized string          `json:"normalized"`
	Input      json.RawMessage `json:"input"`
}

func TestRecipeSHA256_MatchesCheckedInVectors(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", "github", "testdata", "frameworks-recipe-vectors.json"))
	require.NoError(t, err)
	var file struct {
		Vectors []recipeVector `json:"vectors"`
	}
	require.NoError(t, json.Unmarshal(raw, &file))
	names := make([]string, 0, len(file.Vectors))
	for _, vector := range file.Vectors {
		names = append(names, vector.Name)
	}
	require.ElementsMatch(t, []string{"step_snapshot", "step_parse", "step_relate", "step_chunk", "step_embed", "step_build", "step_evaluate", "encoding_edges", "negative_zero"}, names)
	for _, vector := range file.Vectors {
		t.Run(vector.Name, func(t *testing.T) {
			var value any
			require.NoError(t, json.Unmarshal(vector.Input, &value))
			normalized, err := json.Marshal(value)
			require.NoError(t, err)
			require.Equal(t, vector.Normalized, string(normalized))
			require.Equal(t, vector.SHA256, digest(normalized))
			require.Equal(t, vector.SHA256, RecipeSHA256(value))
			require.Equal(t, vector.SHA256, RecipeSHA256(vector.Input))
			if strings.HasPrefix(vector.Name, "step_") {
				var step recipe
				require.NoError(t, json.Unmarshal(vector.Input, &step))
				require.Equal(t, vector.SHA256, RecipeSHA256(step))
			}
		})
	}
}
