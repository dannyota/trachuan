package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"unicode/utf8"
)

// BodyLimit is the maximum request body size in bytes.
const BodyLimit = 65536

func decodeRequest(r *http.Request, hash string) (string, response) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return "", failure(415, "unsupported_media_type", nil)
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, BodyLimit+1))
	if len(data) > BodyLimit {
		return "", failure(413, "body_too_large", nil)
	}
	var timeout net.Error
	if errors.As(err, &timeout) && timeout.Timeout() {
		return "", failure(504, "deadline_exceeded", nil)
	}
	if err != nil || !utf8.Valid(data) || !uniqueJSON(data) {
		return "", failure(400, "invalid_request", nil)
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil || len(raw) != 2 || raw["recipe_hash"] == nil || raw["texts"] == nil {
		return "", failure(400, "invalid_request", nil)
	}
	var recipe string
	var texts []json.RawMessage
	if json.Unmarshal(raw["recipe_hash"], &recipe) != nil || string(raw["recipe_hash"]) == "null" || json.Unmarshal(raw["texts"], &texts) != nil || len(texts) != 1 {
		return "", failure(400, "invalid_request", nil)
	}
	var text string
	if json.Unmarshal(texts[0], &text) != nil || string(texts[0]) == "null" || strings.TrimSpace(text) == "" {
		return "", failure(400, "invalid_request", nil)
	}
	if recipe != hash {
		return "", failure(409, "recipe_mismatch", nil)
	}
	return text, response{}
}
func uniqueJSON(data []byte) bool {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if walkJSON(dec) != nil {
		return false
	}
	_, err := dec.Token()
	return err == io.EOF
}
func walkJSON(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for dec.More() {
			key, keyErr := dec.Token()
			if keyErr != nil {
				return keyErr
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return errInvalidJSON{}
			}
			seen[name] = true
			if walkErr := walkJSON(dec); walkErr != nil {
				return walkErr
			}
		}
	case '[':
		for dec.More() {
			if walkErr := walkJSON(dec); walkErr != nil {
				return walkErr
			}
		}
	default:
		return errInvalidJSON{}
	}
	_, err = dec.Token()
	return err
}

type errInvalidJSON struct{}

func (errInvalidJSON) Error() string { return "invalid JSON" }
