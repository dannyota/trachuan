package serve

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHydrate_ResponseCannotMutateCachedPath(t *testing.T) {
	e := testEngine()
	c := &control{id: 1, versionID: 1, citation: "A-1", path: []string{"A"}}
	controls := map[int64]*control{1: c}
	hits, _, err := e.hydrate(context.Background(), nil, []ranked{{controlID: 1}}, controls, e.project(Caller{}), false)
	require.NoError(t, err)
	hits[0].Path[0] = "changed"
	require.Equal(t, []string{"A"}, c.path)
}
