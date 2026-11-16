//go:build integration

package build

import (
	"context"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/internal/frameworks/parse"
	"danny.vn/trachuan/internal/frameworks/registry"
	"danny.vn/trachuan/internal/frameworks/relate"
	"github.com/stretchr/testify/require"
)

func TestDatabase_RelateEvidenceOnlyUsesLoadedVersions(t *testing.T) {
	base, d := databaseFixture(t)
	reg := &registry.Registry{Frameworks: append(append([]registry.Framework{}, d.versions...), registry.Framework{Code: "fw-beta", Version: "v2", Access: "licensed"})}
	catalogs := map[string]*parse.Result{}
	for _, row := range readRows[catalog](t, filepath.Join(base, "parse/catalogs.jsonl")) {
		for i := range row.Nodes {
			row.Nodes[i].Mappings = nil
			row.Nodes[i].DroppedMappings = 0
		}
		catalogs[versionKey(row.Code, row.Version)] = &parse.Result{Nodes: row.Nodes}
	}
	inputs := []relate.Input{}
	for _, key := range []relate.Key{
		{Code: "fw-beta", Version: "v1", Citation: "A-2"},
		{Code: "fw-beta", Version: "v1", Citation: "missing"},
		{Code: "fw-beta", Version: "v9", Citation: "missing"},
		{Code: "fw-beta", Version: "v2", Citation: "missing"},
	} {
		inputs = append(inputs, relate.Input{From: relate.Key{Code: "fw-alpha", Version: "v1", Citation: "A-2"}, To: key, Relation: "related", Source: "invented", Evidence: "invented licensed evidence"})
	}
	edges, stats, err := relate.Build(reg, catalogs, inputs)
	require.NoError(t, err)
	require.Len(t, edges, 4)
	require.Equal(t, 2, stats.Dropped)
	d.edges = edges
	ctx := context.Background()
	conn := frameworkDB(t)
	require.NoError(t, loadDatabase(ctx, conn, base, "20990101", d, fakeEmbedder{}))
	var mappings, evidence, invalidDependencies int
	require.NoError(t, conn.QueryRow(ctx, `SELECT (SELECT count(*) FROM control_mapping),(SELECT count(*) FROM mapping_evidence),(SELECT count(*) FROM mapping_evidence e CROSS JOIN LATERAL unnest(e.licensed_version_ids) AS dep(id) LEFT JOIN framework_version v ON v.id=dep.id WHERE v.id IS NULL OR v.access<>'licensed')`).Scan(&mappings, &evidence, &invalidDependencies))
	require.Equal(t, 4, mappings)
	require.Equal(t, 2, evidence)
	require.Zero(t, invalidDependencies)
	var dependencies []int64
	require.NoError(t, conn.QueryRow(ctx, `SELECT licensed_version_ids FROM mapping_evidence ORDER BY mapping_id LIMIT 1`).Scan(&dependencies))
	require.Equal(t, []int64{1}, dependencies)
}
