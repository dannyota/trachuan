package serve

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func testEngine() *Engine {
	return &Engine{release: "20990101", key: []byte(strings.Repeat("s", 32)), versions: []version{
		{id: 1, handle: "fw:fw-alpha:2", info: Framework{Code: "fw-alpha", Version: "2", Access: "licensed"}},
		{id: 2, handle: "fw:fw-alpha:1", info: Framework{Code: "fw-alpha", Version: "1", Access: "licensed"}},
		{id: 3, handle: "fw:fw-beta:1", info: Framework{Code: "fw-beta", Version: "1", Access: "free"}},
	}}
}

func fakeCaller(scopes ...string) Caller {
	return Caller{Readable: func(code, access string) bool {
		if access == "free" {
			return true
		}
		if access == "licensed" {
			for _, scope := range scopes {
				if scope == "*" || scope == code {
					return true
				}
			}
		}
		return false
	}}
}

func TestCaller_ServerDecisionContract(t *testing.T) {
	for _, tc := range []struct {
		code, access string
		want         bool
	}{
		{"fw-beta", "free", true}, {"fw-alpha", "licensed", true},
		{"fw-beta", "licensed", false}, {"fw-alpha", "", false}, {"fw-alpha", "unknown", false},
	} {
		require.Equal(t, tc.want, fakeCaller("fw-alpha").Readable(tc.code, tc.access))
	}
}

func TestProjection_BindsCompleteReadableSet(t *testing.T) {
	e := testEngine()
	p := e.project(fakeCaller())
	require.Equal(t, []int64{3}, p.readable)
	require.Equal(t, []int64{1, 2}, p.reduced)
	require.NotEmpty(t, p.version)
	require.Equal(t, p.version, e.project(fakeCaller("fw-absent")).version)
	full := e.project(fakeCaller("fw-alpha"))
	require.Equal(t, []int64{1, 2, 3}, full.readable)
	require.NotEqual(t, p.version, full.version)
	require.Equal(t, full.version, e.project(fakeCaller("*")).version)
	e.versions[0], e.versions[2] = e.versions[2], e.versions[0]
	require.Equal(t, full.version, e.project(fakeCaller("*")).version)
	e.release = "20990102"
	require.NotEqual(t, full.version, e.project(fakeCaller("*")).version)
	require.Empty(t, e.project(Caller{}).readable)
}
