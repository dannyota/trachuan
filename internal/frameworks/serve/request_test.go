package serve

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequest_ValidationAndFilters(t *testing.T) {
	e := testEngine()
	for _, request := range []Request{
		{Query: ""}, {Query: " \n"}, {Query: "a\x00b"}, {Query: "\xff"}, {Query: strings.Repeat("a", 2001)},
		{Query: "access", Limit: -1}, {Query: "access", Limit: 51}, {Query: "access", Mode: "ann"},
		{Query: "access", Filters: map[string][]string{"unknown": {"x"}}},
		{Query: "access", Filters: map[string][]string{"frameworks": {"fw-absent"}}},
		{Query: "access", Filters: map[string][]string{"versions": {"fw:fw-alpha:9"}}},
	} {
		_, err := e.validate(request)
		require.ErrorIs(t, err, ErrInvalidRequest)
		require.Equal(t, "invalid_request", err.Error())
	}
	r, err := e.validate(Request{Query: "Access control", Filters: map[string][]string{"frameworks": {"fw-beta", "fw-alpha", "fw-beta"}}})
	require.NoError(t, err)
	require.Equal(t, []string{"fw-alpha", "fw-beta"}, r.Filters["frameworks"])
	require.Equal(t, 20, r.Limit)
	require.Equal(t, "exact", r.Mode)
	require.Equal(t, "Access control", r.Query)
	_, err = e.validate(Request{Query: "access", Release: "20990102"})
	require.ErrorIs(t, err, ErrReleaseExpired)
}

func TestRequest_IntersectionAndCitationRouting(t *testing.T) {
	e := testEngine()
	e.versions[0].aliases = []string{"Alpha Standard"}
	r, err := e.validate(Request{Query: "alpha standard a.5 (1)", Filters: map[string][]string{"frameworks": {"fw-alpha"}, "versions": {"fw:fw-beta:1"}}})
	require.NoError(t, err)
	require.Empty(t, e.selectVersions(r))
	require.Equal(t, []citationRoute{{"fw-alpha", "A.5(1)"}}, e.citations(r.Query))
	require.Equal(t, []citationRoute{{"fw-alpha", "A.5.1"}}, e.citations("FW-ALPHA A.5.1"))
	require.Empty(t, e.citations("questions about FW-ALPHA A.5.1"))
}
