package build

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		os.Exit(2)
	}
	dir, err := os.MkdirTemp("", "framework-bridge-")
	if err != nil {
		os.Exit(2)
	}
	binary := filepath.Join(dir, "bridge.test")
	command := exec.Command("go", "test", "-c", "-o", binary, "./apps/corpus/internal/cli")
	command.Dir = filepath.Clean(filepath.Join(filepath.Dir(file), "../../.."))
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	command.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off")
	if err = command.Run(); err != nil {
		_ = os.RemoveAll(dir)
		os.Exit(2)
	}
	if err = os.Setenv("FRAMEWORKS_TEST_BRIDGE_BINARY", binary); err != nil {
		_ = os.RemoveAll(dir)
		os.Exit(2)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

type bridgeRequest struct {
	Op, Dir, Key, Hash string
	Chunks             []Chunk
	Reject             map[int]string
}

func bridge(ctx context.Context, req bridgeRequest) (EmbedResult, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return EmbedResult{}, err
	}
	command := exec.CommandContext(ctx, os.Getenv("FRAMEWORKS_TEST_BRIDGE_BINARY"), "-test.run=^TestFrameworksBridge$")
	command.Env = append(os.Environ(), "FRAMEWORKS_TEST_BRIDGE_MODE=1")
	command.Stdin = bytes.NewReader(data)
	out, err := command.Output()
	if err != nil {
		return EmbedResult{}, err
	}
	if req.Op == "validate" {
		return EmbedResult{}, nil
	}
	var result EmbedResult
	err = json.Unmarshal(out, &result)
	return result, err
}

type fakeEmbedder struct{}

func (fakeEmbedder) Recipe() string              { return "test-stage-recipe" }
func (fakeEmbedder) Check(context.Context) error { return nil }
func (fakeEmbedder) Build(ctx context.Context, key string, chunks []Chunk, hash string) (EmbedResult, error) {
	return bridge(ctx, bridgeRequest{Op: "build", Key: key, Chunks: chunks, Hash: hash})
}
func (fakeEmbedder) Validate(ctx context.Context, dir, key string, chunks []Chunk, hash string) error {
	_, err := bridge(ctx, bridgeRequest{Op: "validate", Dir: dir, Key: key, Chunks: chunks, Hash: hash})
	return err
}
func testRun(ctx context.Context, rel string, out io.Writer) error {
	return Run(ctx, rel, out, fakeEmbedder{})
}

type rejectingEmbedder struct {
	calls   int
	checked int
}

func (*rejectingEmbedder) Recipe() string                { return "test-stage-recipe" }
func (e *rejectingEmbedder) Check(context.Context) error { e.checked++; return nil }
func (e *rejectingEmbedder) Build(ctx context.Context, key string, rows []Chunk, hash string) (EmbedResult, error) {
	reject := map[int]string{}
	if len(rows) > 0 {
		reject[rows[0].Ordinal] = "embedding_input_too_long"
	}
	if len(rows) > 1 {
		reject[rows[1].Ordinal] = "embedding_input_bytes_exceeded"
	}
	e.calls++
	return bridge(ctx, bridgeRequest{Op: "build", Key: key, Chunks: rows, Hash: hash, Reject: reject})
}
func (*rejectingEmbedder) Validate(ctx context.Context, dir, key string, chunks []Chunk, hash string) error {
	return (fakeEmbedder{}).Validate(ctx, dir, key, chunks, hash)
}

func TestEmbed_GapsAndResume(t *testing.T) {
	work := fixture(t)
	e := &rejectingEmbedder{}
	var out bytes.Buffer
	require.NoError(t, Run(context.Background(), "20990101", &out, e))
	require.Equal(t, 1, e.checked)
	require.Equal(t, 4, e.calls)
	base := filepath.Join(work, "20990101")
	gaps := readRows[Gap](t, filepath.Join(base, "embed/gaps.jsonl"))
	require.NotEmpty(t, gaps)
	kinds := map[string]bool{}
	for _, gap := range gaps {
		require.NotEmpty(t, gap.Code)
		require.Equal(t, "v1", gap.Version)
		require.NotNil(t, gap.Citation)
		require.Positive(t, gap.Count)
		kinds[gap.Kind] = true
	}
	require.True(t, kinds["embedding_input_too_long"])
	require.True(t, kinds["embedding_input_bytes_exceeded"])
	require.NoError(t, Run(context.Background(), "20990101", &out, e))
	require.Equal(t, 1, e.checked)
	require.Equal(t, 4, e.calls)
	matches, err := filepath.Glob(filepath.Join(base, "embed/artifacts/*/vectors.f32"))
	require.NoError(t, err)
	require.Len(t, matches, 4)
	require.NoError(t, os.WriteFile(matches[0], []byte("corrupt"), 0600))
	require.ErrorContains(t, Run(context.Background(), "20990101", &out, e), "embed: output validation failed")
}

type failedEmbedder struct{ builds int }

func (*failedEmbedder) Validate(context.Context, string, string, []Chunk, string) error { return nil }
func (*failedEmbedder) Recipe() string                                                  { return "test-stage-recipe" }
func (*failedEmbedder) Check(context.Context) error                                     { return fmt.Errorf("recipe mismatch") }
func (e *failedEmbedder) Build(context.Context, string, []Chunk, string) (EmbedResult, error) {
	e.builds++
	return EmbedResult{}, nil
}

func TestEmbed_FailedCheckLeavesNoStep(t *testing.T) {
	work := fixture(t)
	e := &failedEmbedder{}
	require.ErrorContains(t, Run(context.Background(), "20990101", &bytes.Buffer{}, e), "embed: recipe mismatch")
	require.Zero(t, e.builds)
	base := filepath.Join(work, "20990101")
	require.NoDirExists(t, filepath.Join(base, "embed"))
	temp, err := filepath.Glob(filepath.Join(base, ".tmp-embed-*"))
	require.NoError(t, err)
	require.Len(t, temp, 1)
	require.NoError(t, testRun(context.Background(), "20990101", &bytes.Buffer{}))
	require.NoDirExists(t, temp[0])
}

func TestEmbed_ChangedRecipeStopsResume(t *testing.T) {
	for _, field := range []string{"embedding_recipe_hash", "law_stage_recipe_hash"} {
		t.Run(field, func(t *testing.T) {
			work := fixture(t)
			require.NoError(t, testRun(context.Background(), "20990101", &bytes.Buffer{}))
			path := filepath.Join(work, "20990101/embed/manifest.json")
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			var m manifest
			require.NoError(t, json.Unmarshal(data, &m))
			m.Recipe.Parameters[field] = strings.Repeat("0", 64)
			data, err = json.Marshal(m)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, data, 0600))
			require.ErrorContains(t, testRun(context.Background(), "20990101", &bytes.Buffer{}), "embed: inputs or recipe changed")
		})
	}
}

type invalidEmbedder struct{ fakeEmbedder }

func (invalidEmbedder) Build(ctx context.Context, key string, rows []Chunk, hash string) (EmbedResult, error) {
	result, err := (fakeEmbedder{}).Build(ctx, key, rows, hash)
	if err == nil {
		result.Vectors = []byte{0}
	}
	return result, err
}

func TestEmbed_InvalidArtifactIsNotPublished(t *testing.T) {
	work := fixture(t)
	err := Run(context.Background(), "20990101", &bytes.Buffer{}, invalidEmbedder{})
	require.ErrorContains(t, err, "invalid embedding artifact")
	require.NoDirExists(t, filepath.Join(work, "20990101/embed"))
}

type sameControlEmbedder struct{}

func (sameControlEmbedder) Recipe() string              { return "test-stage-recipe" }
func (sameControlEmbedder) Check(context.Context) error { return nil }
func (sameControlEmbedder) Build(ctx context.Context, key string, rows []Chunk, hash string) (EmbedResult, error) {
	reject := map[int]string{}
	for _, row := range rows {
		if row.Citation == "A-2" {
			reject[row.Ordinal] = "embedding_input_too_long"
		}
	}
	return bridge(ctx, bridgeRequest{Op: "build", Key: key, Chunks: rows, Hash: hash, Reject: reject})
}
func (sameControlEmbedder) Validate(ctx context.Context, dir, key string, chunks []Chunk, hash string) error {
	return (fakeEmbedder{}).Validate(ctx, dir, key, chunks, hash)
}

func TestEmbed_CountsRejectedChunksAcrossIndexes(t *testing.T) {
	work := fixture(t)
	require.NoError(t, Run(context.Background(), "20990101", &bytes.Buffer{}, sameControlEmbedder{}))
	base := filepath.Join(work, "20990101")
	chunks := readRows[Chunk](t, filepath.Join(base, "chunk/chunks.jsonl"))
	expected := 0
	for _, c := range chunks {
		if strings.Contains(c.DocumentKey, "fw-alpha:v1") && c.Citation == "A-2" {
			expected++
		}
	}
	gaps := readRows[Gap](t, filepath.Join(base, "embed/gaps.jsonl"))
	var matching []Gap
	for _, gap := range gaps {
		if gap.Code == "fw-alpha" && *gap.Citation == "A-2" {
			matching = append(matching, gap)
		}
	}
	require.Len(t, matching, 1)
	require.Equal(t, expected, matching[0].Count)
}

func TestEmbed_RejectsHashConsistentInvalidArtifact(t *testing.T) {
	for _, scenario := range []string{"invalid index", "vector size", "input hash", "unknown ordinal", "unknown rejection", "invalid dimensions", "missing row", "chunk hash"} {
		t.Run(scenario, func(t *testing.T) {
			work := fixture(t)
			require.NoError(t, testRun(context.Background(), "20990101", &bytes.Buffer{}))
			dir := filepath.Join(work, "20990101/embed")
			paths, err := filepath.Glob(filepath.Join(dir, "artifacts/*/index.json"))
			require.NoError(t, err)
			var chosen string
			var idx map[string]any
			for _, path := range paths {
				data, readErr := os.ReadFile(path)
				require.NoError(t, readErr)
				require.NoError(t, json.Unmarshal(data, &idx))
				if len(idx["rows"].([]any)) > 0 {
					chosen = path
					break
				}
			}
			require.NotEmpty(t, chosen)
			rows := idx["rows"].([]any)
			switch scenario {
			case "invalid index":
				idx["rows"] = 1
			case "vector size":
				require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(chosen), "vectors.f32"), []byte{0}, 0600))
			case "input hash":
				rows[0].(map[string]any)["input_sha256"] = strings.Repeat("0", 64)
			case "unknown ordinal":
				rows[0].(map[string]any)["chunk_ordinal"] = 1000
			case "unknown rejection":
				idx["rejected"] = []any{map[string]any{"chunk_ordinal": 999, "token_count": 9999, "kind": "embedding_input_too_long"}}
			case "invalid dimensions":
				idx["dimensions"] = 1
			case "missing row":
				idx["rows"] = []any{}
				require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(chosen), "vectors.f32"), nil, 0600))
			case "chunk hash":
				idx["chunk_sha256"] = strings.Repeat("0", 64)
			}
			data, err := json.Marshal(idx)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(chosen, data, 0600))
			var lines []string
			for _, name := range []string{"index.json", "report.json", "vectors.f32"} {
				contents, readErr := os.ReadFile(filepath.Join(filepath.Dir(chosen), name))
				require.NoError(t, readErr)
				lines = append(lines, fmt.Sprintf("%s  %s\n", digest(contents), name))
			}
			sort.Strings(lines)
			require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(chosen), "MANIFEST.sha256"), []byte(strings.Join(lines, "")), 0600))
			manifestPath := filepath.Join(dir, "manifest.json")
			data, err = os.ReadFile(manifestPath)
			require.NoError(t, err)
			var m manifest
			require.NoError(t, json.Unmarshal(data, &m))
			m.Outputs, err = outputHashes(dir)
			require.NoError(t, err)
			data, err = json.Marshal(m)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(manifestPath, data, 0600))
			err = testRun(context.Background(), "20990101", &bytes.Buffer{})
			require.ErrorContains(t, err, "invalid embedding artifact")
			require.NotContains(t, err.Error(), "private body")
		})
	}
}
