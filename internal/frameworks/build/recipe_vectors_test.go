package build

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/stretchr/testify/require"
)

type recipeVector struct {
	Name             string          `json:"name"`
	SHA256           string          `json:"sha256"`
	NormalizedBase64 string          `json:"normalized_base64"`
	Value            json.RawMessage `json:"value"`
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
			require.NoError(t, json.Unmarshal(vector.Value, &value))
			normalized, err := json.Marshal(value)
			require.NoError(t, err)
			want, err := base64.StdEncoding.Strict().DecodeString(vector.NormalizedBase64)
			require.NoError(t, err)
			require.Equal(t, string(want), string(normalized))
			require.Equal(t, vector.SHA256, digest(normalized))
			require.Equal(t, vector.SHA256, RecipeSHA256(value))
			require.Equal(t, vector.SHA256, RecipeSHA256(vector.Value))
			if strings.HasPrefix(vector.Name, "step_") {
				var step recipe
				require.NoError(t, json.Unmarshal(vector.Value, &step))
				require.Equal(t, vector.SHA256, RecipeSHA256(step))
			}
		})
	}
}

func TestReleaseRecipes_PassSigningShapes(t *testing.T) {
	directory, err := filepath.Abs(filepath.Join("..", "..", "..", "deploy", "github"))
	require.NoError(t, err)
	recipes, err := json.Marshal(map[string]any{
		"dense": denseRecipe(), "chunk": stepRecipe("chunk"), "lexical": lexical.Recipe1(),
		"retrieval": measuredRetrievalRecipe(0.5, strings.Repeat("a", 64)),
	})
	require.NoError(t, err)
	script := `import sys
sys.path.insert(0,sys.argv[1])
from frameworks_recipes import decode
from frameworks_recipe_shapes import check_recipe_shapes
check_recipe_shapes(decode(sys.stdin.buffer.read()))
`
	cmd := exec.CommandContext(t.Context(), "python3", "-I", "-B", "-c", script, directory)
	cmd.Stdin = bytes.NewReader(recipes)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
}
