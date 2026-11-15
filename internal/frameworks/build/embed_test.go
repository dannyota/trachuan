package build

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeEmbedder struct{}

func (fakeEmbedder) Check(context.Context) error { return nil }
func (fakeEmbedder) Build(_ context.Context, key string, chunks []Chunk, hash string) (EmbedResult, error) {
	data, err := json.Marshal(struct {
		Key  string `json:"document_key"`
		Hash string `json:"chunk_sha256"`
		Rows int    `json:"rows"`
	}{key, hash, len(chunks)})
	return EmbedResult{Index: data, Rows: len(chunks)}, err
}
func testRun(ctx context.Context, rel string, out io.Writer) error {
	return Run(ctx, rel, out, fakeEmbedder{})
}

type rejectingEmbedder struct {
	calls   int
	checked int
}

func (e *rejectingEmbedder) Check(context.Context) error { e.checked++; return nil }
func (e *rejectingEmbedder) Build(ctx context.Context, key string, rows []Chunk, hash string) (EmbedResult, error) {
	result, err := (fakeEmbedder{}).Build(ctx, key, rows, hash)
	if len(rows) > 0 {
		n := 999
		result.Rejected = append(result.Rejected, EmbedRejection{Ordinal: rows[0].Ordinal, Kind: "embedding_input_too_long", TokenCount: &n})
		if len(rows) > 1 {
			result.Rejected = append(result.Rejected, EmbedRejection{Ordinal: rows[1].Ordinal, Kind: "embedding_input_bytes_exceeded"})
		}
	}
	e.calls++
	return result, err
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

func (*failedEmbedder) Check(context.Context) error { return fmt.Errorf("recipe mismatch") }
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
	work := fixture(t)
	require.NoError(t, testRun(context.Background(), "20990101", &bytes.Buffer{}))
	path := filepath.Join(work, "20990101/embed/manifest.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var m manifest
	require.NoError(t, json.Unmarshal(data, &m))
	m.Recipe.Parameters["embedding_recipe_hash"] = strings.Repeat("0", 64)
	data, err = json.Marshal(m)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0600))
	require.ErrorContains(t, testRun(context.Background(), "20990101", &bytes.Buffer{}), "embed: inputs or recipe changed")
}

type sameControlEmbedder struct{}

func (sameControlEmbedder) Check(context.Context) error { return nil }
func (sameControlEmbedder) Build(ctx context.Context, key string, rows []Chunk, hash string) (EmbedResult, error) {
	result, err := (fakeEmbedder{}).Build(ctx, key, rows, hash)
	for _, row := range rows {
		if row.Citation == "A-2" {
			n := 999
			result.Rejected = append(result.Rejected, EmbedRejection{Ordinal: row.Ordinal, Kind: "embedding_input_too_long", TokenCount: &n})
		}
	}
	return result, err
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
