//go:build integration

package build

import (
	"context"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/internal/frameworks/parse"
	"danny.vn/trachuan/internal/frameworks/relate"
	"github.com/stretchr/testify/require"
)

func TestDatabase_LoadsEveryValidRelateEdge(t *testing.T) {
	base, d := databaseFixture(t)
	setRelateMappings(t, base, []parse.Mapping{
		{TargetFramework: "fw-beta", TargetVersion: "v1", TargetCitation: "A-2", Relation: "related"},
		{TargetFramework: "fw-beta", TargetVersion: "v1", TargetCitation: "Z-9", Relation: "related"},
		{TargetFramework: "fw-beta", TargetVersion: "v1", TargetCitation: "Z-8", Relation: "related"},
	})
	ctx := context.Background()
	dir := t.TempDir()
	require.NoError(t, relateStep(ctx, base, dir))
	d.edges = readRows[relate.Edge](t, filepath.Join(dir, "edges.jsonl"))
	require.Len(t, d.edges, 3)
	conn := frameworkDB(t)
	require.NoError(t, loadDatabase(ctx, conn, base, "20990101", d, fakeEmbedder{}))
	var count int
	require.NoError(t, conn.QueryRow(ctx, `SELECT count(*) FROM control_mapping`).Scan(&count))
	require.Equal(t, len(d.edges), count)
	for _, edge := range d.edges {
		var resolved bool
		require.NoError(t, conn.QueryRow(ctx, `SELECT resolved FROM control_mapping WHERE from_code=$1 AND from_version=$2 AND from_citation=$3 AND to_code=$4 AND to_version=$5 AND to_citation=$6 AND relation=$7 AND source=$8`, edge.From.Code, edge.From.Version, edge.From.Citation, edge.To.Code, edge.To.Version, edge.To.Citation, edge.Relation, edge.Source).Scan(&resolved))
		require.Equal(t, edge.Resolved, resolved)
	}
}
