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
	var out []byte
	cursor := 0
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
			out = append(out, input[cursor:start]...)
			out = append(out, cut...)
			cursor = end
		}
	}
	if cursor == 0 {
		return input, nil
	}
	out = append(out, input[cursor:]...)
	return out, nil
}

func cutViewCount(data []byte) ([]byte, bool, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	if _, err := dec.Token(); err != nil {
		return nil, false, fmt.Errorf("decode data object: %w", err)
	}
	before := int(dec.InputOffset())
	cursor := 0
	var out []byte
	changed := false
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
		if key == "viewCount" {
			start := before
			for start < len(data) && (isJSONSpace(data[start]) || data[start] == ',') {
				start++
			}
			out = append(out, data[cursor:start]...)
			next := end
			for next < len(data) && isJSONSpace(data[next]) {
				next++
			}
			if next < len(data) && data[next] == ',' {
				cursor = next + 1
			} else {
				last := len(out) - 1
				for last >= 0 && isJSONSpace(out[last]) {
					last--
				}
				if last >= 0 && out[last] == ',' {
					out = out[:last]
				}
				cursor = end
			}
			changed = true
		}
		before = end
	}
	if _, err := dec.Token(); err != nil {
		return nil, false, fmt.Errorf("decode data object end: %w", err)
	}
	if !changed {
		return data, false, nil
	}
	out = append(out, data[cursor:]...)
	return out, true, nil
}

func isJSONSpace(b byte) bool {
	return b == ' ' || b == '\n' || b == '\r' || b == '\t'
}
