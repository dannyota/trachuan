package serve

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMappings_ValidatesSelectorsRelationsAndLimits(t *testing.T) {
	e := testEngine()
	for _, request := range []MappingsRequest{
		{}, {Handle: "fw:fw-alpha:2"}, {Handle: "bad"},
		{From: "fw:fw-alpha:2"}, {From: "fw:fw-alpha:2#A-1", To: "fw:fw-beta:1"},
		{Handle: "fw:fw-alpha:2#A-1", From: "fw:fw-alpha:2", To: "fw:fw-beta:1"},
		{Handle: "fw:fw-alpha:2#A-1", Limit: -1}, {Handle: "fw:fw-alpha:2#A-1", Limit: 201},
		{Handle: "fw:fw-alpha:2#A-1", Relations: []string{"unknown"}},
	} {
		_, err := e.validateMappings(request)
		require.Error(t, err)
	}
	r, err := e.validateMappings(MappingsRequest{From: "fw:fw-alpha:2", To: "fw:fw-beta:1", Relations: []string{"related", "equivalent", "related"}})
	require.NoError(t, err)
	require.Equal(t, 50, r.Limit)
	require.Equal(t, []string{"equivalent", "related"}, r.Relations)
	r.Limit = 200
	_, err = e.validateMappings(r)
	require.NoError(t, err)
}

func TestRead_ItemErrorsDoNotFailTheCall(t *testing.T) {
	e := testEngine()
	r, err := e.Read(context.Background(), ReadRequest{Items: []ReadItem{{}, {Handle: "bad"}, {Next: "bad"}, {Handle: strings.Repeat("x", 513)}, {Handle: "fw:fw-alpha:2", Next: "bad"}}}, fakeCaller())
	require.NoError(t, err)
	require.Len(t, r.Items, 5)
	for i, want := range []string{"invalid_item", "invalid_handle", "invalid_next", "invalid_item", "invalid_item"} {
		require.Equal(t, want, r.Items[i].Error)
		require.False(t, r.Items[i].Complete)
	}
	require.Equal(t, e.project(fakeCaller()).version, r.AccessSetVersion)
	_, err = e.Read(context.Background(), ReadRequest{Items: []ReadItem{{Handle: "fw:fw-alpha:2"}, {Handle: "vn:vbpl:invented#dieu-1"}}}, fakeCaller())
	require.ErrorIs(t, err, ErrInvalidRequest)
}

func TestRead_MalformedFrameworkItemDoesNotMakeMixedCorpusCall(t *testing.T) {
	r, err := testEngine().Read(context.Background(), ReadRequest{Items: []ReadItem{{Handle: "fw:bad"}, {Handle: "vn:vbpl:invented#dieu-1"}}}, fakeCaller())
	require.NoError(t, err)
	require.Len(t, r.Items, 2)
	require.Equal(t, "invalid_handle", r.Items[0].Error)
	require.Equal(t, "invalid_handle", r.Items[1].Error)
}

func TestFilters_EmptySetsHaveOneContinuationEncoding(t *testing.T) {
	e := testEngine()
	a, err := e.validateMappings(MappingsRequest{Handle: "fw:fw-alpha:2#A-1"})
	require.NoError(t, err)
	b, err := e.validateMappings(MappingsRequest{Handle: "fw:fw-alpha:2#A-1", Relations: []string{}})
	require.NoError(t, err)
	require.Equal(t, mappingDigest(a), mappingDigest(b))
	r, _, err := e.validateGaps(GapsRequest{Versions: []string{}})
	require.NoError(t, err)
	require.Nil(t, r.Versions)
}

func TestRead_NextBindsHandleModeReleaseAndProjection(t *testing.T) {
	e := testEngine()
	p := e.project(fakeCaller())
	c, code := e.readCursor(ReadItem{Handle: "fw:fw-beta:1#A-1"}, "", p)
	require.Empty(t, code)
	require.Equal(t, "control", c.Mode)
	item := ReadItem{Next: e.encodeContinuation(c)}
	_, code = e.readCursor(item, "", p)
	require.Empty(t, code)
	_, code = e.readCursor(item, "", e.project(fakeCaller("*")))
	require.Equal(t, "invalid_next", code)
	c.Release = "20990102"
	_, code = e.readCursor(ReadItem{Next: e.encodeContinuation(c)}, "", p)
	require.Equal(t, "release_expired", code)
	c.Release, c.Mode = e.release, "version"
	_, code = e.readCursor(ReadItem{Next: e.encodeContinuation(c)}, "", p)
	require.Equal(t, "invalid_next", code)
}

func TestQualityGaps_ValidatesVersionFiltersAndKind(t *testing.T) {
	e := testEngine()
	for _, request := range []GapsRequest{{Versions: []string{"fw:fw-alpha:99"}}, {Versions: []string{"fw:fw-alpha:2#A-1"}}, {Kind: "unknown"}} {
		_, _, err := e.validateGaps(request)
		require.ErrorIs(t, err, ErrInvalidRequest)
	}
	r, ids, err := e.validateGaps(GapsRequest{Versions: []string{"fw:fw-beta:1", "fw:fw-alpha:2", "fw:fw-beta:1"}, Kind: "text_incomplete"})
	require.NoError(t, err)
	require.Equal(t, []string{"fw:fw-alpha:2", "fw:fw-beta:1"}, r.Versions)
	require.ElementsMatch(t, []int64{1, 3}, ids)
}

func TestMappings_EvidenceUsesVerbatimCutAndNoHiddenIndicator(t *testing.T) {
	edge := MappingEdge{Evidence: strings.Repeat("invented ", 300)}
	cut := boundedEvidence(edge)
	require.NotNil(t, cut.EvidenceTruncated)
	require.True(t, *cut.EvidenceTruncated)
	require.LessOrEqual(t, len([]rune(cut.Evidence)), 2000)
	require.True(t, strings.HasPrefix(edge.Evidence, cut.Evidence))
	blank := boundedEvidence(MappingEdge{})
	require.Nil(t, blank.EvidenceTruncated)
	require.Empty(t, blank.Evidence)
}

func TestDiagnostics_ValidateReleaseAndPreserveProjectionOnFailure(t *testing.T) {
	e := testEngine()
	e.recipe = rankRecipe()
	caller := fakeCaller()
	caller.AccessUnavailable = true
	status, err := e.CorpusStatus(context.Background(), StatusRequest{}, caller)
	require.ErrorIs(t, err, ErrUnavailable)
	require.Equal(t, "placeholder", status.RetrievalRecipeStatus)
	require.Equal(t, e.project(caller).version, status.AccessSetVersion)
	require.True(t, status.AccessUnavailable)
	_, err = e.CorpusStatus(context.Background(), StatusRequest{Release: "20990102"}, caller)
	require.ErrorIs(t, err, ErrReleaseExpired)
	gaps, err := e.QualityGaps(context.Background(), GapsRequest{Kind: "unknown"}, caller)
	require.ErrorIs(t, err, ErrInvalidRequest)
	require.Equal(t, "placeholder", gaps.RetrievalRecipeStatus)
	require.Equal(t, e.project(caller).version, gaps.AccessSetVersion)
}
