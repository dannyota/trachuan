package build

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/internal/frameworks/relate"
	"github.com/stretchr/testify/require"
)

func TestRelate_LocalLinksAreCountedAsDropped(t *testing.T) {
	work := fixture(t)
	require.NoError(t, testRun(context.Background(), "20990101", &bytes.Buffer{}))
	edges := readRows[relate.Edge](t, filepath.Join(work, "20990101/relate/edges.jsonl"))
	require.Empty(t, edges)
	gaps := readRows[Gap](t, filepath.Join(work, "20990101/relate/gaps.jsonl"))
	require.Len(t, gaps, 2)
	for _, g := range gaps {
		require.Equal(t, "mapping_dropped", g.Kind)
		require.Equal(t, 1, g.Count)
	}
}

func TestReleaseMappings_AggregatesPublicGapCounts(t *testing.T) {
	from := relate.Key{Code: "fw-alpha", Version: "v1", Citation: "A.1"}
	edges, gaps := releaseMappings([]relate.Edge{
		{From: from, To: relate.Key{Code: "fw-alpha", Version: "v1", Citation: "A.2"}},
		{From: from, To: relate.Key{Code: "fw-beta", Version: "v1", Citation: "B.1"}},
		{From: from, To: relate.Key{Code: "fw-beta", Version: "v1", Citation: "B.2"}},
	}, []Gap{{Kind: "mapping_dropped", Code: "fw-alpha", Version: "v1", Count: 2}})
	require.Len(t, edges, 2)
	require.Len(t, gaps, 2)
	for _, gap := range gaps {
		require.Equal(t, "fw-alpha", gap.Code)
		if gap.Kind == "mapping_dropped" {
			require.Equal(t, 3, gap.Count)
			require.Nil(t, gap.Citation)
		} else {
			require.Equal(t, "mapping_unresolved", gap.Kind)
			require.Equal(t, 2, gap.Count)
			require.Equal(t, "A.1", *gap.Citation)
		}
	}
}
