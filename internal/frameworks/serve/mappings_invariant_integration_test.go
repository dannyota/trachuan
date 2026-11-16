//go:build integration

package serve

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func assertMappingsInvariant(t *testing.T, pair operationTwin) {
	t.Helper()
	for _, request := range []MappingsRequest{
		{Handle: "fw:fw-alpha:2#A-1"},
		{From: "fw:fw-alpha:2", To: "fw:fw-beta:1", Limit: 200},
		{Handle: "fw:fw-beta:1#A-1", Relations: []string{"equivalent"}},
		{Handle: "fw:fw-gamma:1#A-1"},
		{Handle: "fw:fw-alpha:2#A-1", Relations: []string{"invalid"}},
		{Handle: "fw:fw-alpha:2#A-1", Limit: 201},
	} {
		seen := map[string]bool{}
		for page := 0; ; page++ {
			require.Less(t, page, 10)
			pair.reset()
			a, ea := pair.a.Mappings(context.Background(), request, pair.caller)
			b, eb := pair.b.Mappings(context.Background(), request, pair.caller)
			pair.compare(t, a, b, ea, eb)
			if request.Limit == 201 || len(request.Relations) > 0 && request.Relations[0] == "invalid" {
				require.ErrorIs(t, ea, ErrInvalidRequest)
				break
			}
			require.NoError(t, ea)
			limit := request.Limit
			if limit == 0 {
				limit = 50
			}
			require.LessOrEqual(t, len(a.Edges), limit)
			for _, edge := range a.Edges {
				require.False(t, seen[edge.Source])
				seen[edge.Source] = true
				if strings.HasPrefix(edge.Source, "invented-001-") || strings.HasPrefix(edge.Source, "invented-002-") {
					require.Empty(t, edge.Evidence)
					require.Nil(t, edge.EvidenceTruncated)
				}
				if edge.Source == "invented-public" {
					require.Equal(t, "Invented public citation alignment.", edge.Evidence)
				}
				if !edge.Resolved {
					require.Equal(t, "fw-delta", edge.To.Code)
					require.Equal(t, "1", edge.To.Version)
					require.Equal(t, "X-1", edge.To.Citation)
					require.Empty(t, edge.To.Title)
					require.Empty(t, edge.To.Handle)
				}
			}
			if a.NextCursor == "" {
				want := 206
				if request.From != "" {
					want = 205
				} else if request.Handle == "fw:fw-gamma:1#A-1" || len(request.Relations) > 0 {
					want = 1
				}
				require.Len(t, seen, want)
				break
			}
			require.Len(t, a.Edges, limit)
			request.Cursor = a.NextCursor
		}
	}
}
