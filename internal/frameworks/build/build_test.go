package build

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	home := filepath.Join(root, "home")
	require.NoError(t, os.Mkdir(home, 0700))
	t.Setenv("HOME", home)
	source := filepath.Join(root, "source")
	work := filepath.Join(root, "work")
	require.NoError(t, os.MkdirAll(filepath.Join(source, "trachuan"), 0700))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "sources"), 0700))
	t.Setenv("FRAMEWORKS_SOURCE_DIR", source)
	t.Setenv("FRAMEWORKS_WORK_DIR", work)
	t.Setenv("TMPDIR", root)
	var registry strings.Builder
	registry.WriteString("frameworks:\n")
	for i, code := range []string{"fw-alpha", "fw-beta"} {
		data := []byte(fmt.Sprintf(`{"catalog":{"groups":[{"id":"a","title":"publisher family %s","controls":[{"id":"a-2","title":"publisher private title","parts":[{"prose":%q}],"links":[{"rel":"related","href":"#aa-9"}],"controls":[{"id":"a-2.1","props":[{"name":"label","value":"A-2(1)"}],"title":"publisher child","parts":[{"prose":"child body"}]}]}]}]}}`, code, strings.Repeat("private body ", 180)))
		name := code + ".json"
		require.NoError(t, os.WriteFile(filepath.Join(source, name), data, 0600))
		access := "licensed"
		if i == 1 {
			access = "free"
		}
		//nolint:misspell // Registry fields follow the design.
		fmt.Fprintf(&registry, "  - code: %s\n    version: v1\n    name: Framework %d\n    publisher: Invented\n    language: en\n    access: %s\n    licence_kind: public-domain\n    licence_url: https://example.invalid/license\n    ordinal: 1\n    citation_scheme: oscal-catalog\n    parser: nist-oscal-800-53\n    expected_controls: 3\n    files:\n      - path: %s\n        sha256: %x\n        size: %d\n        role: catalog\n        format: json\n", code, i, access, name, sha256.Sum256(data), len(data))
	}
	require.NoError(t, os.WriteFile("sources/frameworks.yaml", []byte(registry.String()), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(source, "trachuan/titles.csv"), []byte("code,version,citation,title\nfw-alpha,v1,A-2,Our title\n"), 0600))
	return work
}

func readRows[T any](t *testing.T, path string) []T {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var rows []T
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var row T
		require.NoError(t, json.Unmarshal(line, &row))
		rows = append(rows, row)
	}
	return rows
}

func TestRun_ArtifactsAndResume(t *testing.T) {
	work := fixture(t)
	var out bytes.Buffer
	require.NoError(t, testRun(context.Background(), "20990101", &out))
	require.Contains(t, out.String(), "build complete")
	for _, forbidden := range []string{"private body", "publisher", "Our title", "A-2"} {
		require.NotContains(t, out.String(), forbidden)
	}
	base := filepath.Join(work, "20990101")
	controls := readRows[Control](t, filepath.Join(base, "parse/controls.jsonl"))
	require.Len(t, controls, 6)
	require.Nil(t, controls[0].Title)
	require.Equal(t, "Our title", *controls[1].Title)
	require.Equal(t, "publisher private title", controls[1].TitleOriginal)
	require.Equal(t, "publisher family fw-beta", *controls[3].Title)
	gaps := readRows[Gap](t, filepath.Join(base, "parse/gaps.jsonl"))
	require.Len(t, gaps, 2)
	require.Equal(t, "title_missing", gaps[0].Kind)
	edges := readRows[map[string]any](t, filepath.Join(base, "relate/edges.jsonl"))
	require.Empty(t, edges)
	chunks := readRows[Chunk](t, filepath.Join(base, "chunk/chunks.jsonl"))
	last := map[string]int{}
	counts := map[string]int{}
	var bodyPieces []string
	for _, chunk := range chunks {
		if prior, ok := last[chunk.DocumentKey]; ok {
			require.Greater(t, chunk.Ordinal, prior)
		}
		last[chunk.DocumentKey] = chunk.Ordinal
		counts[chunk.DocumentKey]++
		if chunk.Index == "public" && chunk.DocumentKey == "fwp:fw-alpha:v1" {
			require.NotContains(t, chunk.Content, "publisher")
			require.NotContains(t, chunk.Content, "body")
		}
		if chunk.Index == "body" && chunk.Citation == "A-2(1)" && chunk.DocumentKey == "fw:fw-alpha:v1" {
			require.Equal(t, "Framework 0 v1\nA > A-2 > A-2(1)", chunk.ContextPrefix)
		}
		if chunk.Index == "body" && chunk.Citation == "A-2" && chunk.DocumentKey == "fw:fw-alpha:v1" {
			require.Equal(t, "Framework 0 v1\nA > A-2\nOur title", chunk.ContextPrefix)
			require.Equal(t, len(bodyPieces), chunk.ChunkIndex)
			bodyPieces = append(bodyPieces, chunk.Content)
		}
	}
	require.Equal(t, 3, counts["fwp:fw-alpha:v1"])
	require.Equal(t, 4, counts["fw:fw-alpha:v1"])
	require.Equal(t, strings.TrimSpace(strings.Repeat("private body ", 180)), strings.Join(bodyPieces, " "))
	before, err := os.Stat(filepath.Join(base, "chunk/manifest.json"))
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(os.Getenv("FRAMEWORKS_SOURCE_DIR")))
	require.NoError(t, os.Remove("sources/frameworks.yaml"))
	out.Reset()
	require.NoError(t, testRun(context.Background(), "20990101", &out))
	require.Equal(t, 7, strings.Count(out.String(), "skipped"))
	after, err := os.Stat(filepath.Join(base, "chunk/manifest.json"))
	require.NoError(t, err)
	require.Equal(t, before.ModTime(), after.ModTime())
}

func TestRun_Determinism(t *testing.T) {
	work := fixture(t)
	for _, rel := range []string{"20990101", "20990102"} {
		require.NoError(t, testRun(context.Background(), rel, &bytes.Buffer{}))
	}
	require.NoError(t, filepath.WalkDir(filepath.Join(work, "20990101"), func(path string, entry os.DirEntry, err error) error {
		require.NoError(t, err)
		if entry.IsDir() || entry.Name() == ".lock" || strings.Contains(path, string(filepath.Separator)+"evaluate"+string(filepath.Separator)) {
			return nil
		}
		a, err := os.ReadFile(path)
		require.NoError(t, err)
		b, err := os.ReadFile(strings.Replace(path, "20990101", "20990102", 1))
		require.NoError(t, err)
		require.Equal(t, string(a), string(b), path)
		return nil
	}))
	a := readRows[EvaluationReport](t, filepath.Join(work, "20990101/evaluate/report.json"))[0]
	b := readRows[EvaluationReport](t, filepath.Join(work, "20990102/evaluate/report.json"))[0]
	require.Equal(t, a.EvaluationResult, b.EvaluationResult)
	require.Equal(t, a.QualityPass, b.QualityPass)
	require.Equal(t, a.Floors, b.Floors)
}

func TestRun_LockAndCleanup(t *testing.T) {
	work := fixture(t)
	base := filepath.Join(work, "20990101")
	require.NoError(t, os.MkdirAll(filepath.Join(base, ".tmp-parse-leftover"), 0700))
	f, err := os.OpenFile(filepath.Join(base, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close()) })
	require.NoError(t, unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB))
	require.ErrorContains(t, testRun(context.Background(), "20990101", &bytes.Buffer{}), "locked")
	require.DirExists(t, filepath.Join(base, ".tmp-parse-leftover"))
	require.NoError(t, unix.Flock(int(f.Fd()), unix.LOCK_UN))
	require.NoError(t, testRun(context.Background(), "20990101", &bytes.Buffer{}))
	require.NoDirExists(t, filepath.Join(base, ".tmp-parse-leftover"))
}

func TestRun_Validation(t *testing.T) {
	for _, rel := range []string{"../20990101", "20990101_100", "2099-01-01", "20990101_", ""} {
		t.Run(rel, func(t *testing.T) {
			fixture(t)
			require.Error(t, testRun(context.Background(), rel, &bytes.Buffer{}))
		})
	}
	t.Run("corrupt output", func(t *testing.T) {
		work := fixture(t)
		require.NoError(t, testRun(context.Background(), "20990101_1", &bytes.Buffer{}))
		require.NoError(t, os.WriteFile(filepath.Join(work, "20990101_1/parse/controls.jsonl"), []byte("private corrupt citation"), 0600))
		err := testRun(context.Background(), "20990101_1", &bytes.Buffer{})
		require.ErrorContains(t, err, "parse")
		require.NotContains(t, err.Error(), "private corrupt citation")
	})
}
