package cli

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

func sourceSnapshot(data []byte) (json.RawMessage, error) {
	var list any
	if err := yaml.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parse source list: %w", err)
	}
	result, err := json.Marshal(list)
	if err != nil {
		return nil, fmt.Errorf("encode source list: %w", err)
	}
	return result, nil
}
