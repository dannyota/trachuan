package build

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRun_ChangedInputStops(t *testing.T) {
	work := fixture(t)
	require.NoError(t, testRun(context.Background(), "20990101", &bytes.Buffer{}))
	dir := filepath.Join(work, "20990101/snapshot")
	path := filepath.Join(dir, "trachuan/titles.csv")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data = append(data, []byte("fw-beta,v1,A,Another title\n")...)
	require.NoError(t, os.WriteFile(path, data, 0600))
	manifestPath := filepath.Join(dir, "manifest.json")
	encoded, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	var m manifest
	require.NoError(t, json.Unmarshal(encoded, &m))
	hash := digest(data)
	m.Inputs["trachuan/titles.csv"] = &hash
	m.Outputs["trachuan/titles.csv"] = hash
	encoded, err = json.Marshal(m)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(manifestPath, append(encoded, '\n'), 0600))
	err = testRun(context.Background(), "20990101", &bytes.Buffer{})
	require.ErrorContains(t, err, "parse: inputs or recipe changed")
}

func TestRun_InvalidExistingStepStops(t *testing.T) {
	for _, scenario := range []string{"missing manifest", "recipe", "extra output", "symlink"} {
		t.Run(scenario, func(t *testing.T) {
			work := fixture(t)
			require.NoError(t, testRun(context.Background(), "20990101", &bytes.Buffer{}))
			dir := filepath.Join(work, "20990101/chunk")
			switch scenario {
			case "missing manifest":
				require.NoError(t, os.Remove(filepath.Join(dir, "manifest.json")))
			case "recipe":
				path := filepath.Join(dir, "manifest.json")
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				data = bytes.Replace(data, []byte(`"limit":512`), []byte(`"limit":256`), 1)
				require.NoError(t, os.WriteFile(path, data, 0600))
			case "extra output":
				require.NoError(t, os.WriteFile(filepath.Join(dir, "extra"), []byte("unexpected"), 0600))
			case "symlink":
				path := filepath.Join(dir, "chunks.jsonl")
				require.NoError(t, os.Rename(path, path+".old"))
				require.NoError(t, os.Symlink(path+".old", path))
			}
			require.ErrorContains(t, testRun(context.Background(), "20990101", &bytes.Buffer{}), "chunk")
		})
	}
}

func TestRun_FailedStepLeavesTemporaryDirectory(t *testing.T) {
	work := fixture(t)
	path := filepath.Join(os.Getenv("FRAMEWORKS_SOURCE_DIR"), "trachuan/titles.csv")
	require.NoError(t, os.WriteFile(path, []byte("code,version,citation,title\nfw-alpha,v1,A,private\nfw-alpha,v1,A,duplicate\n"), 0600))
	require.ErrorContains(t, testRun(context.Background(), "20990101", &bytes.Buffer{}), "parse")
	base := filepath.Join(work, "20990101")
	require.NoDirExists(t, filepath.Join(base, "parse"))
	matches, err := filepath.Glob(filepath.Join(base, ".tmp-parse-*"))
	require.NoError(t, err)
	require.Len(t, matches, 1)
	require.DirExists(t, filepath.Join(base, "snapshot"))
}

func TestRun_OptionalInputs(t *testing.T) {
	work := fixture(t)
	source := os.Getenv("FRAMEWORKS_SOURCE_DIR")
	require.NoError(t, os.Remove(filepath.Join(source, "trachuan/titles.csv")))
	require.NoError(t, os.WriteFile(filepath.Join(source, "trachuan/ocr-waivers.yaml"), []byte("waivers: []\n"), 0600))
	require.NoError(t, testRun(context.Background(), "20990101", &bytes.Buffer{}))
	data, err := os.ReadFile(filepath.Join(work, "20990101/snapshot/manifest.json"))
	require.NoError(t, err)
	var m manifest
	require.NoError(t, json.Unmarshal(data, &m))
	require.Contains(t, m.Inputs, "trachuan/titles.csv")
	require.Nil(t, m.Inputs["trachuan/titles.csv"])
	require.NotNil(t, m.Inputs["trachuan/ocr-waivers.yaml"])
	controls := readRows[Control](t, filepath.Join(work, "20990101/parse/controls.jsonl"))
	require.Nil(t, controls[1].Title)
	require.Equal(t, "publisher private title", *controls[4].Title)
}

func TestSnapshotInputs_RefusesSymlinkDirectory(t *testing.T) {
	work := fixture(t)
	require.NoError(t, testRun(context.Background(), "20990101", &bytes.Buffer{}))
	dir := filepath.Join(work, "20990101/snapshot")
	require.NoError(t, os.Rename(filepath.Join(dir, "sources"), filepath.Join(work, "moved")))
	require.NoError(t, os.Symlink(filepath.Join(work, "moved"), filepath.Join(dir, "sources")))
	_, err := snapshotInputs(dir)
	require.Error(t, err)
}
