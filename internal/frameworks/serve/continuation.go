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

type continuation struct {
	Operation string   `json:"operation"`
	Release   string   `json:"release"`
	Access    string   `json:"access"`
	Digest    string   `json:"digest"`
	Key       []string `json:"key,omitempty"`
	Handle    string   `json:"handle,omitempty"`
	Mode      string   `json:"mode,omitempty"`
	Unit      int      `json:"unit,omitempty"`
	Part      int      `json:"part,omitempty"`
}

func digestValue(value any) string {
	raw, _ := json.Marshal(value)
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

func (e *Engine) encodeContinuation(c continuation) string {
	raw, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + e.sign("frameworks-cursor", raw)
}

func (e *Engine) unpackContinuation(token string) (continuation, error) {
	encoded, signature, ok := strings.Cut(token, ".")
	raw, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if !ok || err != nil || !hmac.Equal([]byte(signature), []byte(e.sign("frameworks-cursor", raw))) {
		return continuation{}, ErrInvalidRequest
	}
	var c continuation
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&c) != nil || dec.Decode(new(any)) != io.EOF || c.Release == "" || c.Access == "" || c.Digest == "" || c.Unit < 0 || c.Part < 0 {
		return continuation{}, ErrInvalidRequest
	}
	return c, nil
}

func (e *Engine) decodeContinuation(token, operation, digest string, p projection) (continuation, error) {
	c, err := e.unpackContinuation(token)
	invalid := error(ErrInvalidRequest)
	expired := error(ErrCursorExpired)
	if operation == "read" {
		invalid, expired = ErrInvalidNext, ErrReleaseExpired
	}
	if err != nil || c.Operation != operation {
		return continuation{}, invalid
	}
	if c.Release != e.release {
		return continuation{}, expired
	}
	if c.Access != p.version {
		if operation == "read" {
			return continuation{}, ErrInvalidNext
		}
		return continuation{}, ErrCursorExpired
	}
	if c.Digest != digest {
		return continuation{}, invalid
	}
	return c, nil
}
