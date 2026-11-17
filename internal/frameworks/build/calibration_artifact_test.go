package build

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCalibration_PublishesMeasuredRecipeAndHashedRecord(t *testing.T) {
	dir := t.TempDir()
	db := &releaseRecorder{}
	e := calibrationEngine{map[string]calibrationAnswer{
		"private invented positive": {cosine: cosine(.8)}, "private invented negative": {cosine: cosine(.2)},
	}}
	r, err := CalibrateCases(context.Background(), e, calibrationCases(), strings.Repeat("b", 64))
	require.NoError(t, err)
	hash, recipe, err := persistCalibration(context.Background(), db, dir, r)
	require.NoError(t, err)
	raw, err := os.ReadFile(filepath.Join(dir, "calibration.json"))
	require.NoError(t, err)
	require.Equal(t, digest(raw), hash)
	require.Equal(t, "measured", recipe["status"])
	require.Equal(t, hash, recipe["abstention"].(map[string]any)["calibration_sha256"])
	require.Equal(t, recipe, db.args[0])
	require.NotContains(t, string(raw), "private invented")
	info, err := os.Stat(filepath.Join(dir, "calibration.json"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestBuild_PlaceholderHasNoCalibration(t *testing.T) {
	work := fixture(t)
	goldenInput(t, "floors.yaml", "opaque: private floors\n")
	require.NoError(t, testRun(context.Background(), "20990101", os.Stderr))
	dir := filepath.Join(work, "20990101/build")
	raw, err := os.ReadFile(filepath.Join(dir, "build.json"))
	require.NoError(t, err)
	var m buildManifest
	require.NoError(t, json.Unmarshal(raw, &m))
	require.Empty(t, m.CalibrationSHA256)
	require.Equal(t, "placeholder", m.RetrievalRecipe["status"])
	require.NoFileExists(t, filepath.Join(dir, "calibration.json"))
	require.NoError(t, validateBuild(dir))
}
