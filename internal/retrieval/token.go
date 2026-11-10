package retrieval

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
)

// ReadItem describes one document, provision, range, relation list or continuation.
type ReadItem struct {
	Handle    string `json:"handle,omitempty"`
	Number    string `json:"number,omitempty"`
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
	Relations bool   `json:"relations,omitempty"`
	Next      string `json:"next,omitempty"`
}

type cursor struct {
	V       int
	Release string
	Item    ReadItem
	Unit    int
	Part    int
}

var errInvalidNext = errors.New("invalid next token")

func encodeNext(c cursor) string {
	data, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeNext(s string) (cursor, error) {
	data, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return cursor{}, errInvalidNext
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var c cursor
	if err := dec.Decode(&c); err != nil {
		return cursor{}, errInvalidNext
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return cursor{}, errInvalidNext
	}
	if c.V != 1 || c.Unit < 0 || c.Part < 0 || c.Item.Next != "" || (c.Item.Handle != "" && c.Item.Number != "") {
		return cursor{}, errInvalidNext
	}
	return c, nil
}
