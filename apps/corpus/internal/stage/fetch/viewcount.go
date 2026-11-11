package fetch

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func removeViewCount(input []byte) ([]byte, error) {
	var valid json.RawMessage
	if err := json.Unmarshal(input, &valid); err != nil {
		return nil, fmt.Errorf("decode detail: %w", err)
	}
	if len(valid) == 0 || valid[0] != '{' {
		return input, nil
	}
	dec := json.NewDecoder(bytes.NewReader(input))
	_, _ = dec.Token()
	for dec.More() {
		key, _ := dec.Token()
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, fmt.Errorf("decode detail member: %w", err)
		}
		if key != "data" || len(value) == 0 || value[0] != '{' {
			continue
		}
		end := int(dec.InputOffset())
		start := end - len(value)
		cut, changed, err := cutViewCount(value)
		if err != nil {
			return nil, fmt.Errorf("cut view count: %w", err)
		}
		if changed {
			result := make([]byte, 0, len(input)-len(value)+len(cut))
			result = append(result, input[:start]...)
			result = append(result, cut...)
			result = append(result, input[end:]...)
			return removeViewCount(result)
		}
	}
	return input, nil
}

func cutViewCount(data []byte) ([]byte, bool, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	if _, err := dec.Token(); err != nil {
		return nil, false, fmt.Errorf("decode data object: %w", err)
	}
	before := int(dec.InputOffset())
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return nil, false, fmt.Errorf("decode data key: %w", err)
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, false, fmt.Errorf("decode data value: %w", err)
		}
		end := int(dec.InputOffset())
		if key != "viewCount" {
			before = end
			continue
		}
		start := before
		for start < len(data) && isJSONSpace(data[start]) {
			start++
		}
		comma := -1
		if start < len(data) && data[start] == ',' {
			comma = start
			start++
		}
		for start < len(data) && isJSONSpace(data[start]) {
			start++
		}
		next := end
		for next < len(data) && isJSONSpace(data[next]) {
			next++
		}
		if next < len(data) && data[next] == ',' {
			end = next + 1
		} else if comma >= 0 {
			start = comma
		}
		result := make([]byte, 0, len(data)-(end-start))
		result = append(result, data[:start]...)
		result = append(result, data[end:]...)
		return result, true, nil
	}
	if _, err := dec.Token(); err != nil {
		return nil, false, fmt.Errorf("decode data object end: %w", err)
	}
	return data, false, nil
}

func isJSONSpace(b byte) bool {
	return b == ' ' || b == '\n' || b == '\r' || b == '\t'
}
