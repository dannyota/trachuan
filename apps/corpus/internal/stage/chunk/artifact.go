// Package chunk creates provision-owned retrieval chunks.
package chunk

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Node is an extracted provision and its position in the source tree.
type Node struct {
	NodeID       string  `json:"node_id"`
	ParentNodeID *string `json:"parent_node_id"`
	Kind         string  `json:"kind"`
	Label        string  `json:"label"`
	Heading      string  `json:"heading"`
	Ordinal      int     `json:"ordinal"`
	Path         string  `json:"path"`
	Text         string  `json:"text"`
}

// Meta holds document fields used by the chunk recipe.
type Meta struct {
	Key, Number, Title, EffFrom, IssueDate string
}

// File is the chunks.json artifact.
type File struct {
	Version       int     `json:"version"`
	DocumentKey   string  `json:"document_key"`
	FetchSHA256   string  `json:"fetch_sha256"`
	ExtractSHA256 string  `json:"extract_sha256"`
	Chunks        []Chunk `json:"chunks"`
}

// Chunk holds rendered source text and its owning provision.
type Chunk struct {
	Ordinal       int    `json:"ordinal"`
	NodeID        string `json:"node_id"`
	Anchor        string `json:"anchor"`
	ContextPrefix string `json:"context_prefix"`
	Content       string `json:"content"`
	RoughTokens   int    `json:"rough_tokens"`
	Spans         []Span `json:"spans"`
}

// Span maps an interval of source text to an identical content interval.
type Span struct {
	NodeID       string `json:"node_id"`
	Start        int    `json:"start"`
	End          int    `json:"end"`
	ContentStart int    `json:"content_start"`
	ContentEnd   int    `json:"content_end"`
	Role         string `json:"role"`
}

// Gap describes an incomplete chunk artifact.
type Gap struct {
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}

// Range identifies an interval of an extracted provision.
type Range struct {
	NodeID string `json:"node_id"`
	Start  int    `json:"start"`
	End    int    `json:"end"`
}

// Omission identifies a source interval excluded from the output.
type Omission struct {
	NodeID      string `json:"node_id"`
	Start       int    `json:"start"`
	End         int    `json:"end"`
	Reason      string `json:"reason"`
	KeptOrdinal *int   `json:"kept_ordinal"`
}

// Coverage is the coverage.json artifact.
type Coverage struct {
	Version          int        `json:"version"`
	DocumentKey      string     `json:"document_key"`
	Gaps             []Gap      `json:"gaps"`
	Omissions        []Omission `json:"omissions"`
	CoveredRanges    []Range    `json:"covered_ranges"`
	WhitespaceRanges []Range    `json:"whitespace_ranges"`
}

// ReadArtifact decodes both artifact files without verifying their manifest.
func ReadArtifact(dir string) (File, Coverage, error) {
	var file File
	var coverage Coverage
	for _, entry := range []struct {
		name  string
		value any
	}{{"chunks.json", &file}, {"coverage.json", &coverage}} {
		data, err := os.ReadFile(filepath.Join(dir, entry.name))
		if err != nil {
			return File{}, Coverage{}, fmt.Errorf("read %s: %w", entry.name, err)
		}
		if err := json.Unmarshal(data, entry.value); err != nil {
			return File{}, Coverage{}, fmt.Errorf("decode %s: %w", entry.name, err)
		}
	}
	return file, coverage, nil
}
