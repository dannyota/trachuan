package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFrameworks_Usage(t *testing.T) {
	for _, args := range [][]string{{"frameworks"}, {"frameworks", "build"}, {"frameworks", "embed", "20990101"}, {"frameworks", "build", "20990101", "extra"}} {
		var out, errout bytes.Buffer
		require.Equal(t, 2, Run(context.Background(), args, &out, &errout))
		require.Contains(t, errout.String(), "corpus frameworks build <rel>")
	}
}

func TestFrameworks_BuildAndNoTextOutput(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	require.NoError(t, os.MkdirAll("sources", 0700))
	require.NoError(t, os.MkdirAll("source", 0700))
	require.NoError(t, os.MkdirAll("home", 0700))
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("TMPDIR", root)
	t.Setenv("FRAMEWORKS_SOURCE_DIR", filepath.Join(root, "source"))
	t.Setenv("FRAMEWORKS_WORK_DIR", filepath.Join(root, "work"))
	data := []byte(`{"catalog":{"groups":[{"id":"aa","title":"licensed title"}]}}`)
	require.NoError(t, os.WriteFile("source/catalog.json", data, 0600))
	registry := fmt.Sprintf("frameworks:\n  - code: fw-alpha\n    version: v1\n    name: Invented\n    publisher: Invented\n    language: en\n    access: licensed\n    ordinal: 1\n    citation_scheme: oscal-catalog\n    parser: nist-oscal-800-53\n    expected_controls: 1\n    files:\n      - path: catalog.json\n        sha256: %x\n        size: %d\n        role: catalog\n        format: json\n", sha256.Sum256(data), len(data))
	require.NoError(t, os.WriteFile("sources/frameworks.yaml", []byte(registry), 0600))
	var out, errout bytes.Buffer
	require.Equal(t, 0, Run(context.Background(), []string{"frameworks", "build", "20990101"}, &out, &errout), errout.String())
	require.Contains(t, out.String(), "chunk complete")
	require.NotContains(t, out.String()+errout.String(), "licensed title")
	require.FileExists(t, "work/20990101/chunk/manifest.json")
	require.NoDirExists(t, "work/20990101/embed")
	require.NoError(t, os.WriteFile("work/20990101/parse/controls.jsonl", []byte("licensed text AA"), 0600))
	out.Reset()
	errout.Reset()
	require.Equal(t, 1, Run(context.Background(), []string{"frameworks", "build", "20990101"}, &out, &errout))
	require.Contains(t, errout.String(), "parse")
	require.NotContains(t, out.String()+errout.String(), "licensed text")
}
