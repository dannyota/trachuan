package relate

import (
	"testing"

	"danny.vn/trachuan/internal/frameworks/parse"
	"danny.vn/trachuan/internal/frameworks/registry"
	"github.com/stretchr/testify/require"
)

func TestBuild_EvidenceRequiresReleaseVersions(t *testing.T) {
	for _, tc := range []struct {
		name, version, citation, nonRelease, evidence string
		resolved, kept                                bool
		dropped                                       int
	}{
		{"unknown version without evidence", "9", "B", "", "", false, false, 0},
		{"unknown version with evidence", "9", "B", "", "invented private evidence", false, false, 1},
		{"registered non-release target", "2", "B", "", "invented private evidence", false, false, 1},
		{"non-release source", "1", "B", "source", "invented private evidence", true, false, 1},
		{"two release versions", "1", "B", "", "invented private evidence", true, true, 0},
		{"missing citation in release", "1", "missing", "", "invented private evidence", false, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reg := &registry.Registry{Frameworks: []registry.Framework{
				{Code: "alpha", Version: "1", Parser: "nist-oscal-800-53", Access: "licensed"},
				{Code: "beta", Version: "1", Parser: "nist-oscal-800-53", Access: "free"},
				{Code: "beta", Version: "2"},
			}}
			if tc.nonRelease == "source" {
				reg.Frameworks[0].Parser = ""
			}
			catalogs := map[string]*parse.Result{"alpha:1": {Nodes: []parse.Node{{Citation: "A"}}}, "beta:1": {Nodes: []parse.Node{{Citation: "B"}}}}
			input := Input{From: Key{"alpha", "1", "A"}, To: Key{"beta", tc.version, tc.citation}, Relation: "related", Source: "invented", Evidence: tc.evidence}
			edges, stats, err := Build(reg, catalogs, []Input{input})
			require.NoError(t, err)
			require.Len(t, edges, 1)
			require.Equal(t, input.To, edges[0].To)
			require.Equal(t, tc.resolved, edges[0].Resolved)
			unresolved := 0
			if !tc.resolved {
				unresolved = 1
			}
			require.Equal(t, unresolved, stats.Unresolved)
			require.Zero(t, stats.Duplicates)
			require.Equal(t, tc.dropped, stats.Dropped)
			if tc.dropped != 0 {
				require.Equal(t, map[VersionKey]int{{Code: "alpha", Version: "1"}: tc.dropped}, stats.DroppedByVersion)
			}
			if tc.kept {
				require.Equal(t, &Evidence{Value: tc.evidence, Requires: []string{"fw:alpha:1"}}, edges[0].Evidence)
			} else {
				require.Nil(t, edges[0].Evidence)
			}
		})
	}
}
