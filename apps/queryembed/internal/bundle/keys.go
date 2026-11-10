package bundle

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// validateKeys checks exact field names before encoding/json can match aliases.
func validateKeys(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	return walkKeys(dec, "manifest")
}

func walkKeys(dec *json.Decoder, kind string) error {
	token, err := dec.Token()
	if err != nil {
		return fmt.Errorf("read JSON: %w", err)
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]bool)
		for dec.More() {
			token, err := dec.Token()
			if err != nil {
				return fmt.Errorf("read object key: %w", err)
			}
			key, ok := token.(string)
			if !ok {
				return fmt.Errorf("invalid object key")
			}
			allowed := kind == "manifest" && (key == "model" || key == "revision" || key == "resolved_revision" || key == "files") || kind == "file" && (key == "path" || key == "size" || key == "sha256")
			if !allowed || seen[key] {
				return fmt.Errorf("unknown or duplicate key %q", key)
			}
			seen[key] = true
			child := ""
			if kind == "manifest" && key == "files" {
				child = "files"
			}
			if err := walkKeys(dec, child); err != nil {
				return err
			}
		}
	case '[':
		child := ""
		if kind == "files" {
			child = "file"
		}
		for dec.More() {
			if err := walkKeys(dec, child); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter")
	}
	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("close JSON value: %w", err)
	}
	return nil
}
