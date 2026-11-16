package cli

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/frameworks/build"
	"github.com/stretchr/testify/require"
)

func TestFrameworkEmbedder_ReadsVerifiedVectorRows(t *testing.T) {
	startFrameworkEmbedder(t)
	ctx := context.Background()
	service := newFrameworkEmbedder()
	chunks := []build.Chunk{{Ordinal: 0, Content: "Invented body", ContextPrefix: "Invented framework"}, {Ordinal: 1, Content: "Invented second body"}}
	hash := strings.Repeat("a", 64)
	result, err := service.Build(ctx, "fw:invented:v1", chunks, hash)
	require.NoError(t, err)
	dir := t.TempDir()
	files := map[string][]byte{"index.json": result.Index, "vectors.f32": result.Vectors, "report.json": []byte(`{"version":1,"rows":2,"rejected":0,"gaps":[]}`)}
	var manifest []string
	for name, data := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0600))
		manifest = append(manifest, fmt.Sprintf("%x  %s\n", sha256.Sum256(data), name))
	}
	sort.Strings(manifest)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "MANIFEST.sha256"), []byte(strings.Join(manifest, "")), 0600))
	rows, err := service.Read(ctx, dir, "fw:invented:v1", chunks, hash)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	for i, row := range rows {
		require.Equal(t, i, row.Ordinal)
		require.Equal(t, 2, row.TokenCount)
		require.Len(t, row.Values, embedding.DocumentRecipe1().Dimensions)
		require.Equal(t, float32(1), row.Values[0])
	}
	_, err = service.Read(ctx, dir, "fw:invented:v2", chunks, hash)
	require.ErrorContains(t, err, "identity mismatch")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vectors.f32"), []byte{0}, 0600))
	_, err = service.Read(ctx, dir, "fw:invented:v1", chunks, hash)
	require.Error(t, err)
}
