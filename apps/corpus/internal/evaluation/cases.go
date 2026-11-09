// Package evaluation scores retrieval responses against evaluation fixtures.
package evaluation

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Expectation describes one legacy citation.
type Expectation struct {
	DocNumber     string   `json:"doc_number"`
	Article       string   `json:"article,omitempty"`
	Clause        string   `json:"clause,omitempty"`
	Point         string   `json:"point,omitempty"`
	AltDocNumbers []string `json:"alt_doc_numbers,omitempty"`
	RelationOK    bool     `json:"relation_ok,omitempty"`
}

// Case describes an authored evaluation case.
type Case struct {
	ID                string        `json:"id"`
	Question          string        `json:"question"`
	ExpectedCitations []Expectation `json:"expected_citations,omitempty"`
	ExpectAbstain     bool          `json:"expect_abstain,omitempty"`
	ExpectFail        bool          `json:"expect_fail,omitempty"`
	Notes             string        `json:"notes,omitempty"`
}

// FixtureHashes identifies the inputs used to construct a suite.
type FixtureHashes struct {
	Legacy      string
	IdentityMap string
	OldProduct  string
	Inventory   string
}

// Suite holds the cases and adjudications for evaluation.
type Suite struct {
	Legacy     []Case
	Map        IdentityMap
	Rejections []string
	Hashes     FixtureHashes
}

// LoadLegacy decodes and validates the authored cases.
func LoadLegacy(r io.Reader) ([]Case, error) {
	var cases []Case
	dec := json.NewDecoder(r)
	if err := dec.Decode(&cases); err != nil {
		return nil, fmt.Errorf("decode legacy cases: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decode legacy cases: trailing JSON")
		}
		return nil, fmt.Errorf("decode legacy cases: %w", err)
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("legacy cases: empty suite")
	}
	seen := make(map[string]bool)
	for _, c := range cases {
		if strings.TrimSpace(c.ID) == "" || seen[c.ID] || strings.TrimSpace(c.Question) == "" {
			return nil, fmt.Errorf("legacy cases: invalid or duplicate ID or empty question")
		}
		seen[c.ID] = true
		if c.ExpectAbstain == (len(c.ExpectedCitations) > 0) {
			return nil, fmt.Errorf("legacy case %q: contradictory or missing expectations", c.ID)
		}
		for _, e := range c.ExpectedCitations {
			if strings.TrimSpace(e.DocNumber) == "" {
				return nil, fmt.Errorf("legacy case %q: empty document number", c.ID)
			}
			for _, n := range e.AltDocNumbers {
				if strings.TrimSpace(n) == "" {
					return nil, fmt.Errorf("legacy case %q: empty alternate number", c.ID)
				}
			}
		}
	}
	return cases, nil
}
