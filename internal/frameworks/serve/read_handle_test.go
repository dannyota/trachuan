package serve

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRead_ContinuationMustMatchRouteHandle(t *testing.T) {
	e := &Engine{release: "20990101", key: []byte("invented-key-with-at-least-32-bytes")}
	p := e.project(Caller{})
	handle := "fw:fw-alpha:1#A"
	c := continuation{Operation: "read", Release: e.release, Access: p.version, Handle: handle, Mode: "control", Digest: digestValue([]string{handle, "control"})}
	response, err := e.Read(t.Context(), ReadRequest{Items: []ReadItem{{Next: e.encodeContinuation(c)}}, ExpectedHandle: "fw:fw-alpha:1#B"}, Caller{})
	require.NoError(t, err)
	require.Len(t, response.Items, 1)
	require.Equal(t, "invalid_next", response.Items[0].Error)
}
