//go:build integration

package serve

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSearch_FilterPreservesHitsAcrossPages(t *testing.T) {
	e, trace := buildFixture(t, false)
	e.recipe.DenseDepth, e.recipe.LexicalDepth = 5, 3
	ctx := context.Background()
	sawCombined := false
	for _, caller := range []Caller{{}, fakeCaller(), fakeCaller("fw-gamma"), fakeCaller("*")} {
		for _, query := range []string{"quartz safeguard", "quartz second", "nebula", "unrelated"} {
			open, err := e.Search(ctx, Request{Query: query, Limit: 50}, caller)
			require.NoError(t, err)
			openPositions := map[string]int{}
			for i, hit := range open.Hits {
				openPositions[hit.Handle] = i
			}
			for _, filters := range []map[string][]string{
				{"frameworks": {"fw-alpha"}},
				{"frameworks": {"fw-beta"}},
				{"versions": {"fw:fw-alpha:1"}},
				{"versions": {"fw:fw-alpha:2", "fw:fw-beta:1"}},
				{"frameworks": {"fw-alpha"}, "versions": {"fw:fw-alpha:1", "fw:fw-beta:1"}},
				{"frameworks": {"fw-beta"}, "versions": {"fw:fw-alpha:2"}},
			} {
				request := Request{Query: query, Limit: 2, Filters: filters}
				var hits []Hit
				for page := 0; ; page++ {
					require.Less(t, page, 20)
					trace.reset()
					response, err := e.Search(ctx, request, caller)
					require.NoError(t, err)
					trace.assertSearchScan(t, e.project(caller))
					if query == "unrelated" {
						require.True(t, response.Abstain)
					}
					hits = append(hits, response.Hits...)
					if response.NextCursor == "" {
						break
					}
					require.NotEmpty(t, response.Hits)
					request.Cursor = response.NextCursor
				}
				positions := map[string]int{}
				prefix, fill := 0, 0
				lastGlobal := -1
				allowed := e.controlsFor(e.selectVersions(request))
				for i, hit := range hits {
					require.NotContains(t, positions, hit.Handle)
					positions[hit.Handle] = i
					c := filterHitControl(t, e, hit)
					require.Contains(t, allowed, c.id)
					if position, ok := openPositions[hit.Handle]; ok {
						require.Zero(t, fill)
						require.Greater(t, position, lastGlobal)
						lastGlobal = position
						require.Equal(t, open.Hits[position], hit)
						prefix++
					} else {
						fill++
					}
				}
				sawCombined = sawCombined || (prefix > 0 && fill > 0)
				for i, hit := range open.Hits {
					c := filterHitControl(t, e, hit)
					if _, ok := allowed[c.id]; ok {
						require.Contains(t, positions, hit.Handle)
						require.LessOrEqual(t, positions[hit.Handle], i)
					}
				}
			}
		}
	}
	require.True(t, sawCombined)
}

func filterHitControl(t *testing.T, e *Engine, hit Hit) *control {
	t.Helper()
	h, err := parseHandle(hit.Handle)
	require.NoError(t, err)
	v, ok := e.versionFor(h)
	require.True(t, ok)
	c := e.partitions[v.id].citations[h.citation]
	require.NotNil(t, c)
	return c
}
