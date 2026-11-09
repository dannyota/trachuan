// Package lexical implements the shared lexical encoding recipe.
package lexical

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
)

// Recipe identifies every part of a lexical encoding.
type Recipe struct {
	Version       int     `json:"version"`
	Algorithm     string  `json:"algorithm"`
	Input         string  `json:"input"`
	Normalization string  `json:"normalization"`
	Tokenizer     string  `json:"tokenizer"`
	HashAlgorithm string  `json:"hash"`
	Dimensions    int     `json:"dimensions"`
	K1            float64 `json:"k1"`
	B             float64 `json:"b"`
	Query         string  `json:"query"`
	Collision     string  `json:"collision"`
}

// Recipe1 returns the baseline lexical recipe.
func Recipe1() Recipe {
	return Recipe{1, "bm25-vn-fold/1", "content-space-prefix", "lower-nfd-mn-dd/1", "unicode-letter-digit/1", "fnv1a32", 1048576, 1.2, 0.75, "hashed-presence", "sum-document"}
}

// JSON returns the compact recipe representation.
func (r Recipe) JSON() ([]byte, error) { return json.Marshal(r) }

// Hash returns the SHA-256 of the compact recipe representation.
func (r Recipe) Hash() string {
	b, err := r.JSON()
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (r Recipe) validate() error {
	if r != Recipe1() {
		return errors.New("unsupported lexical recipe")
	}
	return nil
}
