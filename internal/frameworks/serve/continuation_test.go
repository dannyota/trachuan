package serve

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContinuation_OperationSelectorAndAccessBindings(t *testing.T) {
	e := testEngine()
	p := e.project(fakeCaller())
	for _, op := range []string{"read", "mappings", "quality_gaps"} {
		t.Run(op, func(t *testing.T) {
			c := continuation{Operation: op, Release: e.release, Access: p.version, Digest: digestValue("selector"), Key: []string{"public-key"}}
			token := e.encodeContinuation(c)
			got, err := e.decodeContinuation(token, op, c.Digest, p)
			require.NoError(t, err)
			require.Equal(t, c, got)
			_, err = e.decodeContinuation(token, op, digestValue("changed selector"), p)
			require.Error(t, err)
			want := error(ErrCursorExpired)
			if op == "read" {
				want = ErrInvalidNext
			}
			_, err = e.decodeContinuation(token, op, c.Digest, e.project(fakeCaller("*")))
			require.ErrorIs(t, err, want)
			c.Release = "20990102"
			_, err = e.decodeContinuation(e.encodeContinuation(c), op, c.Digest, p)
			if op == "read" {
				want = ErrReleaseExpired
			}
			require.ErrorIs(t, err, want)
			_, err = e.decodeContinuation(token+"!", op, c.Digest, p)
			require.Error(t, err)
			_, err = e.decodeContinuation(token, "search", c.Digest, p)
			require.ErrorIs(t, err, ErrInvalidRequest)
		})
	}
}
