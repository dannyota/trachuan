package build

import (
	"errors"
	"fmt"
	"os"

	"danny.vn/trachuan/apps/corpus/internal/source"
)

// CorrectionsInput holds correction bytes captured for a build.
type CorrectionsInput struct {
	bytes      []byte
	hash, path string
}

// ReadCorrections loads reviewed correction bytes; a missing file is an empty list.
func ReadCorrections(path string) (CorrectionsInput, error) {
	data, err := readCorrections(path)
	if err != nil {
		return CorrectionsInput{}, err
	}
	return newCorrectionsInput(data, path)
}
func readCorrections(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return []byte("[]\n"), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read corrections: %w", err)
	}
	return data, nil
}
func newCorrectionsInput(data []byte, path string) (CorrectionsInput, error) {
	if _, err := source.ParseCorrections(data); err != nil {
		return CorrectionsInput{}, err
	}
	copyBytes := append([]byte(nil), data...)
	return CorrectionsInput{bytes: copyBytes, hash: hashBytes(copyBytes), path: path}, nil
}
func (c CorrectionsInput) pinned() ([]source.Correction, correctionInput, error) {
	if c.path != "" {
		data, err := readCorrections(c.path)
		if err != nil {
			return nil, correctionInput{}, err
		}
		c.bytes = data
	} else {
		if c.bytes == nil && c.hash == "" {
			c.bytes = []byte("[]\n")
			c.hash = hashBytes(c.bytes)
		}
		if hashBytes(c.bytes) != c.hash {
			return nil, correctionInput{}, fmt.Errorf("correction bytes changed")
		}
	}
	entries, err := source.ParseCorrections(c.bytes)
	if err != nil {
		return nil, correctionInput{}, fmt.Errorf("parse pinned corrections: %w", err)
	}
	canonical, err := canonicalCorrections(entries)
	if err != nil {
		return nil, correctionInput{}, err
	}
	return entries, correctionInput{hash: hashBytes(canonical), nonempty: len(entries) > 0}, nil
}
