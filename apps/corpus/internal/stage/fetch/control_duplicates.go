package fetch

import (
	"bytes"
	"encoding/json"
)

func duplicateMembers(data []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	duplicate, err := walkControlValue(decoder)
	return err != nil || duplicate
}

func walkControlValue(decoder *json.Decoder) (bool, error) {
	token, err := decoder.Token()
	if err != nil {
		return false, err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return false, nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			name, nameErr := decoder.Token()
			if nameErr != nil {
				return false, nameErr
			}
			key := name.(string)
			if _, exists := seen[key]; exists {
				return true, nil
			}
			seen[key] = struct{}{}
			duplicate, walkErr := walkControlValue(decoder)
			if walkErr != nil || duplicate {
				return duplicate, walkErr
			}
		}
	case '[':
		for decoder.More() {
			duplicate, walkErr := walkControlValue(decoder)
			if walkErr != nil || duplicate {
				return duplicate, walkErr
			}
		}
	}
	_, err = decoder.Token()
	return false, err
}
