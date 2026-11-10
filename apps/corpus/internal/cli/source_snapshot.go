package cli

import (
	"encoding/json"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"gopkg.in/yaml.v3"
)

func sourceSnapshot(data, corrections []byte) (json.RawMessage, error) {
	entries, err := source.ParseCorrections(corrections)
	if err != nil {
		return nil, err
	}
	if len(entries) != 0 {
		return nil, fmt.Errorf("validity corrections are not implemented")
	}
	var list map[string]any
	if decodeErr := yaml.Unmarshal(data, &list); decodeErr != nil {
		return nil, fmt.Errorf("parse source list: %w", decodeErr)
	}
	if list == nil {
		return nil, fmt.Errorf("empty source list")
	}
	if _, exists := list["corrections_bytes"]; exists {
		return nil, fmt.Errorf("source list reserves corrections_bytes")
	}
	list["corrections_bytes"] = string(corrections)
	result, err := json.Marshal(list)
	if err != nil {
		return nil, fmt.Errorf("encode source list: %w", err)
	}
	return result, nil
}
