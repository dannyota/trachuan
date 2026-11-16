package serve

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
)

type cursor struct {
	Operation string `json:"operation"`
	Release   string `json:"release"`
	Access    string `json:"access"`
	Digest    string `json:"digest"`
	Position  int    `json:"position"`
}

func requestDigest(r Request) string {
	raw, _ := json.Marshal(struct {
		Query, Mode string
		Filters     map[string][]string
	}{r.Query, r.Mode, r.Filters})
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

func (e *Engine) encodeCursor(r Request, p projection, position int) string {
	raw, _ := json.Marshal(cursor{"search", e.release, p.version, requestDigest(r), position})
	return base64.RawURLEncoding.EncodeToString(raw) + "." + e.sign("frameworks-cursor", raw)
}

func (e *Engine) cursorPosition(token string, r Request, p projection) (int, error) {
	if token == "" {
		return 0, nil
	}
	if len(token) > 2048 {
		return 0, ErrInvalidRequest
	}
	encoded, signature, ok := strings.Cut(token, ".")
	raw, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if !ok || err != nil || !hmac.Equal([]byte(signature), []byte(e.sign("frameworks-cursor", raw))) {
		return 0, ErrInvalidRequest
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var c cursor
	if dec.Decode(&c) != nil || dec.Decode(new(any)) != io.EOF || c.Operation != "search" || c.Release == "" || c.Access == "" || c.Position <= 0 {
		return 0, ErrInvalidRequest
	}
	if c.Release != e.release || c.Access != p.version {
		return 0, ErrCursorExpired
	}
	if c.Digest != requestDigest(r) {
		return 0, ErrInvalidRequest
	}
	return c.Position, nil
}
