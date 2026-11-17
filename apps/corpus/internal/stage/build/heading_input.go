package build

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/source"
)

type HeadingExclusionsInput struct {
	path string
}

func ReadHeadingExclusions(path string) (HeadingExclusionsInput, error) {
	input := HeadingExclusionsInput{path: path}
	if _, _, err := input.pinned(); err != nil {
		return HeadingExclusionsInput{}, err
	}
	return input, nil
}

func (input HeadingExclusionsInput) pinned() ([]source.HeadingExclusion, string, error) {
	data := []byte("[]")
	if input.path != "" {
		var err error
		data, err = os.ReadFile(input.path)
		if err != nil {
			return nil, "", fmt.Errorf("read heading exclusions %q: %w", input.path, err)
		}
	}
	entries, err := source.ParseHeadingExclusions(data)
	if err != nil {
		return nil, "", err
	}
	slices.SortFunc(entries, func(a, b source.HeadingExclusion) int {
		if a.Key != b.Key {
			return strings.Compare(a.Key, b.Key)
		}
		return strings.Compare(a.Anchor, b.Anchor)
	})
	encoded, err := json.Marshal(entries)
	if err != nil {
		return nil, "", fmt.Errorf("encode heading exclusions: %w", err)
	}
	return entries, hashBytes(encoded), nil
}
