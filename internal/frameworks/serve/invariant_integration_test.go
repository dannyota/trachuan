//go:build integration

package serve

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func responseBytes(t *testing.T, response Response, err error) []byte {
	t.Helper()
	message := ""
	if err != nil {
		message = err.Error()
	}
	raw, marshalErr := json.Marshal(struct {
		Response Response `json:"response"`
		Error    string   `json:"error,omitempty"`
	}{response, message})
	require.NoError(t, marshalErr)
	return raw
}

func TestSearch_UnreadableTextInvariant(t *testing.T) {
	a, traceA := buildFixture(t, false)
	b, traceB := buildFixture(t, true)
	assertVersionScan(t, a)
	assertVersionScan(t, b)
	ctx := context.Background()
	var recipeA, recipeB []byte
	require.NoError(t, a.db.QueryRow(ctx, `SELECT retrieval_recipe FROM release`).Scan(&recipeA))
	require.NoError(t, b.db.QueryRow(ctx, `SELECT retrieval_recipe FROM release`).Scan(&recipeB))
	require.Equal(t, recipeA, recipeB)
	var chunksA, chunksB int
	require.NoError(t, a.db.QueryRow(ctx, `SELECT count(*) FROM body_chunk WHERE version_id=2`).Scan(&chunksA))
	require.NoError(t, b.db.QueryRow(ctx, `SELECT count(*) FROM body_chunk WHERE version_id=2`).Scan(&chunksB))
	require.NotEqual(t, chunksA, chunksB, "licensed mutation must change chunk boundaries")
	queries := []Request{
		{Query: "quartz safeguard", Limit: 2},
		{Query: "nebula", Limit: 1},
		{Query: "FW-ALPHA A-1", Limit: 1},
		{Query: "fw-alpha standard A-1", Limit: 1},
		{Query: "FW-ALPHA A-99"},
		{Query: "quartz", Limit: 1, Filters: map[string][]string{"frameworks": {"fw-alpha", "fw-beta"}}},
		{Query: "quartz", Limit: 1, Filters: map[string][]string{"versions": {"fw:fw-alpha:2", "fw:fw-gamma:1"}}},
		{Query: "quartz", Filters: map[string][]string{"frameworks": {"fw-beta"}, "versions": {"fw:fw-alpha:2"}}},
		{Query: "unrelated"},
		{Query: "unrelated", Filters: map[string][]string{"versions": {"fw:fw-alpha:2"}}},
		{Query: "quartz", Filters: map[string][]string{"unknown": {"value"}}},
		{Query: "quartz", Filters: map[string][]string{"frameworks": {"fw-absent"}}},
	}
	for name, caller := range map[string]Caller{
		"empty": {}, "reduced": fakeCaller(), "mixed": fakeCaller("fw-gamma"),
	} {
		t.Run(name, func(t *testing.T) {
			for i, request := range queries {
				seen := map[string]bool{}
				for page := 0; ; page++ {
					require.Less(t, page, 30, "pagination failed to terminate")
					traceA.reset()
					traceB.reset()
					ra, ea := a.Search(ctx, request, caller)
					rb, eb := b.Search(ctx, request, caller)
					require.Equal(t, responseBytes(t, ra, ea), responseBytes(t, rb, eb), "query=%d page=%d", i, page)
					traceA.assertProjection(t, a.project(caller).readable)
					traceB.assertProjection(t, b.project(caller).readable)
					require.Equal(t, traceA.snapshot(), traceB.snapshot(), "projection read pattern differs")
					if i < len(queries)-2 {
						require.NoError(t, ea)
					} else {
						require.ErrorIs(t, ea, ErrInvalidRequest)
					}
					if request.Query == "unrelated" {
						require.True(t, ra.Abstain)
					}
					for _, hit := range ra.Hits {
						require.False(t, seen[hit.Handle], "duplicate across pages")
						seen[hit.Handle] = true
						if hit.TextWithheld {
							require.Empty(t, hit.TitleOriginal)
							require.Empty(t, hit.Snippet)
						}
					}
					if ra.NextCursor == "" {
						break
					}
					require.NotEmpty(t, ra.Hits)
					request.Cursor = ra.NextCursor
				}
			}
		})
	}
	request := Request{Query: "quartz", Limit: 50}
	ra, ea := a.Search(ctx, request, fakeCaller("fw-alpha"))
	rb, eb := b.Search(ctx, request, fakeCaller("fw-alpha"))
	require.NoError(t, ea)
	require.NoError(t, eb)
	require.NotEqual(t, responseBytes(t, ra, ea), responseBytes(t, rb, eb), "entitled positive control must observe the mutation")
	var snippetA, snippetB string
	for _, hit := range ra.Hits {
		if hit.Handle == "fw:fw-alpha:2#A-1" {
			snippetA = hit.Snippet
		}
	}
	for _, hit := range rb.Hits {
		if hit.Handle == "fw:fw-alpha:2#A-1" {
			snippetB = hit.Snippet
		}
	}
	require.NotEmpty(t, snippetA)
	require.NotEmpty(t, snippetB)
	require.NotEqual(t, snippetA, snippetB)
}

func TestSearch_LookupAndCursorRevocation(t *testing.T) {
	e, trace := buildFixture(t, false)
	ctx := context.Background()
	request := Request{Query: "FW-ALPHA A-1", Limit: 1}
	first, err := e.Search(ctx, request, fakeCaller())
	require.NoError(t, err)
	require.Len(t, first.Hits, 1)
	require.Equal(t, "fw:fw-alpha:2#A-1", first.Hits[0].Handle)
	require.NotNil(t, first.Hits[0].Lookup)
	require.True(t, *first.Hits[0].Lookup)
	require.Nil(t, first.Hits[0].Score)
	require.False(t, first.Abstain)
	require.NotEmpty(t, first.NextCursor)
	trace.assertProjection(t, e.project(fakeCaller()).readable)
	request.Cursor = first.NextCursor
	second, err := e.Search(ctx, request, fakeCaller())
	require.NoError(t, err)
	require.Len(t, second.Hits, 1)
	require.Equal(t, "fw:fw-alpha:1#A-1", second.Hits[0].Handle)
	require.Empty(t, second.NextCursor)
	_, err = e.Search(ctx, request, fakeCaller("fw-alpha"))
	require.ErrorIs(t, err, ErrCursorExpired)
	request.Query = "quartz"
	_, err = e.Search(ctx, request, fakeCaller())
	require.ErrorIs(t, err, ErrInvalidRequest)
}
