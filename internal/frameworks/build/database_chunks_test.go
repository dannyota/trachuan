package build

import (
	"testing"

	"danny.vn/trachuan/internal/frameworks/registry"
	"danny.vn/trachuan/internal/frameworks/relate"
	"github.com/stretchr/testify/require"
)

func evidenceFixture() (databaseInputs, databaseIDs, relate.Edge) {
	d := databaseInputs{versions: []registry.Framework{{Code: "fw-alpha", Version: "v1", Access: "licensed"}, {Code: "fw-beta", Version: "v1", Access: "licensed"}}}
	ids := databaseIDs{versions: map[string]int64{"fw-alpha:v1": 2, "fw-beta:v1": 1}}
	edge := relate.Edge{From: relate.Key{Code: "fw-alpha", Version: "v1", Citation: "A.1"}, To: relate.Key{Code: "fw-beta", Version: "v1", Citation: "B.1"}, Evidence: &relate.Evidence{Value: "Invented", Requires: []string{"fw:fw-alpha:v1", "fw:fw-beta:v1"}}}
	return d, ids, edge
}

func TestEvidenceDependencies_RejectsUnloadedVersions(t *testing.T) {
	for _, access := range []string{"free", "licensed"} {
		for end, name := range []string{"from", "to"} {
			t.Run(access+"/"+name, func(t *testing.T) {
				d, ids, edge := evidenceFixture()
				d.versions[end].Access = access
				if access == "free" {
					edge.Evidence.Requires = []string{"fw:fw-beta:v1"}
					if end == 1 {
						edge.Evidence.Requires = []string{"fw:fw-alpha:v1"}
					}
				}
				delete(ids.versions, versionKey(d.versions[end].Code, d.versions[end].Version))
				_, err := evidenceDependencies(d, ids, edge)
				require.Error(t, err)
			})
		}
	}
}

func TestLexicalPostings_UsesLawNormalizationAndTermFrequency(t *testing.T) {
	rows, count, err := lexicalPostings("ĐIỆN điện alpha", "Framework Alpha")
	require.NoError(t, err)
	require.Equal(t, 5, count)
	require.Equal(t, []posting{{"alpha", 2}, {"dien", 2}, {"framework", 1}}, rows)
	rows, count, err = lexicalPostings("...", "")
	require.NoError(t, err)
	require.Zero(t, count)
	require.Empty(t, rows)
}

func TestEvidenceDependencies_FailsClosed(t *testing.T) {
	d, ids, edge := evidenceFixture()
	deps, err := evidenceDependencies(d, ids, edge)
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2}, deps)
	for _, requires := range [][]string{nil, {}, {"fw:fw-alpha:v1"}, {"fw:fw-beta:v1", "fw:fw-alpha:v1"}, {"fw:fw-alpha:v1", "fw:fw-beta:v1", "fw:unknown:v1"}} {
		edge.Evidence.Requires = requires
		_, err = evidenceDependencies(d, ids, edge)
		require.Error(t, err)
	}
	d.versions[0].Access = "free"
	d.versions[1].Access = "free"
	edge.Evidence.Requires = []string{}
	deps, err = evidenceDependencies(d, ids, edge)
	require.NoError(t, err)
	require.Empty(t, deps)
	d.versions[0].Access = ""
	_, err = evidenceDependencies(d, ids, edge)
	require.ErrorContains(t, err, "unknown evidence access")
	d.versions[0].Access = "free"
	d.versions = d.versions[:1]
	_, err = evidenceDependencies(d, ids, edge)
	require.ErrorContains(t, err, "evidence version not in release")
}
