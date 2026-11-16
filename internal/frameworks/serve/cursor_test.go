package serve

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCursor_BindingsAndSignature(t *testing.T) {
	e := testEngine()
	r, err := e.validate(Request{Query: "access", Filters: map[string][]string{"frameworks": {"fw-alpha"}}})
	require.NoError(t, err)
	p := e.project(fakeCaller())
	token := e.encodeCursor(r, p, 7)
	require.Equal(t, token, e.encodeCursor(r, p, 7))
	position, err := e.cursorPosition(token, r, p)
	require.NoError(t, err)
	require.Equal(t, 7, position)
	for _, change := range []func(*Request){
		func(r *Request) { r.Query = "other" },
		func(r *Request) { r.Filters = nil },
		func(r *Request) { r.Mode = "other" },
	} {
		changed := r
		change(&changed)
		_, err = e.cursorPosition(token, changed, p)
		require.ErrorIs(t, err, ErrInvalidRequest)
	}
	_, err = e.cursorPosition(token, r, e.project(fakeCaller("*")))
	require.ErrorIs(t, err, ErrCursorExpired)
	e.release = "20990102"
	_, err = e.cursorPosition(token, r, p)
	require.ErrorIs(t, err, ErrCursorExpired)
	e.release = "20990101"
	e.key[0]++
	_, err = e.cursorPosition(token, r, p)
	require.ErrorIs(t, err, ErrInvalidRequest)
	_, err = e.cursorPosition(token+"!", r, p)
	require.ErrorIs(t, err, ErrInvalidRequest)
}

func TestCursor_RejectsOtherOperations(t *testing.T) {
	e := testEngine()
	r, err := e.validate(Request{Query: "quartz"})
	require.NoError(t, err)
	p := e.project(fakeCaller())
	raw, err := json.Marshal(cursor{"read", e.release, p.version, requestDigest(r), 1})
	require.NoError(t, err)
	token := base64.RawURLEncoding.EncodeToString(raw) + "." + e.sign("frameworks-cursor", raw)
	_, err = e.cursorPosition(token, r, p)
	require.ErrorIs(t, err, ErrInvalidRequest)
}
