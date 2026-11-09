// Package embedding holds the embedder protocol types and recipe reports
// shared by the corpus pipeline and the server.
package embedding

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Recipe is the runtime recipe an embedder reports at GET /v1/recipe. Field
// order matters: the hash is SHA-256 of the compact JSON in this order, the
// same bytes each embedding service hashes.
type Recipe struct {
	Model             string `json:"model"`
	Revision          string `json:"revision"`
	ResolvedRevision  string `json:"resolved_revision"`
	TokenizerRevision string `json:"tokenizer_revision"`
	TokenizerSHA256   string `json:"tokenizer_sha256"`
	WeightsSHA256     string `json:"weights_sha256"`
	Dimensions        int    `json:"dimensions"`
	Pooling           string `json:"pooling"`
	Normalization     string `json:"normalization"`
	Input             string `json:"input"`
	Runtime           string `json:"runtime"`
	Backend           string `json:"backend"`
	Precision         string `json:"precision"`
	TokenCap          int    `json:"token_cap"`
	SpecialTokens     bool   `json:"special_tokens"`
	Truncation        bool   `json:"truncation"`
	Language          string `json:"language"`
	TokenizerLibrary  string `json:"tokenizer_library"`
	RuntimeLibrary    string `json:"runtime_library"`
}

// CompactJSON returns the recipe as compact JSON without HTML escaping.
func (r Recipe) CompactJSON() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(r); err != nil {
		return nil, fmt.Errorf("encode recipe: %w", err)
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// Hash returns the hex SHA-256 of the compact JSON, matching the service.
func (r Recipe) Hash() string {
	data, err := r.CompactJSON()
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Vector is one embedding returned by the service.
type Vector struct {
	Values     []float32
	TokenCount int
	RecipeHash string
}

//go:embed recipes/vi-v2-document.json
var documentRecipe []byte

//go:embed recipes/vi-v2-query.json
var queryRecipe []byte

func mustRecipe(data []byte) Recipe {
	var r Recipe
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		panic(fmt.Sprintf("embedded recipe: %v", err))
	}
	return r
}

// DocumentRecipe1 returns the pinned document runtime recipe (ADR 0009, ADR 0011).
func DocumentRecipe1() Recipe { return mustRecipe(documentRecipe) }

// QueryRecipe1 returns the pinned query runtime recipe (ADR 0009, ADR 0011).
func QueryRecipe1() Recipe { return mustRecipe(queryRecipe) }
