package build

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

type releaseRecorder struct{ args []any }

func (r *releaseRecorder) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	r.args = args
	return pgconn.CommandTag{}, nil
}

func TestBuildRecipe_RejectsPreviousCalibrationPolicy(t *testing.T) {
	base := t.TempDir()
	tmp, target := filepath.Join(base, "tmp"), filepath.Join(base, "build")
	require.NoError(t, os.Mkdir(tmp, 0700))
	require.NoError(t, writeFile(tmp, "artifact", []byte("invented artifact")))
	previous := stepRecipe("build")
	previous.Parameters["calibration"] = "public-free-only-open/1"
	require.NoError(t, seal(tmp, target, previous, nil))
	require.ErrorContains(t, validate(target, stepRecipe("build"), nil), "inputs or recipe changed")
}

func TestRelease_BootstrapRecipe(t *testing.T) {
	r := &releaseRecorder{}
	require.NoError(t, initializeRelease(context.Background(), r, "20990101", nil))
	encoded, err := json.Marshal(r.args)
	require.NoError(t, err)
	var args []json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &args))
	var recipe map[string]any
	for _, raw := range args {
		var candidate map[string]any
		if json.Unmarshal(raw, &candidate) == nil && candidate["dense_depth"] != nil {
			recipe = candidate
		}
	}
	require.NotNil(t, recipe, "release must store a retrieval recipe")
	require.Equal(t, "placeholder", recipe["status"])
	law := retrieval.Recipe3()
	require.Equal(t, float64(law.DenseDepth), recipe["dense_depth"])
	require.Equal(t, float64(law.LexicalDepth), recipe["lexical_depth"])
	require.Equal(t, map[string]any{"k": float64(60), "dense_weight": float64(1), "lexical_weight": 0.5, "routed_lexical_weight": float64(1)}, recipe["rrf"])
	require.Equal(t, map[string]any{"statistic": "first-hit-best-permitted-cosine", "threshold": 0.5297041131124263, "status": "bootstrap-from-law"}, recipe["abstention"])
	require.Equal(t, map[string]any{"release": "vn/20261115_3", "recipe_version": float64(3), "calibration_sha256": "9264612fa2d933e3f0b1ee451e7707598192f9c66ed52224d48a9a327ca65113"}, recipe["source"])
	params, err := json.Marshal(databaseRecipe())
	require.NoError(t, err)
	require.Contains(t, string(params), `"retrieval_recipe":`)
}
