// Package source parses frozen source lists.
package source

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// List holds the sources and code tables for a country.
type List struct {
	Country       string                  `yaml:"country" json:"country"`
	VBPL          VBPLSources             `yaml:"vbpl" json:"vbpl"`
	RelationCodes map[string]RelationCode `yaml:"relation_codes" json:"relation_codes"`
	StatusCodes   map[string]StatusCode   `yaml:"status_codes" json:"status_codes"`
}

// VBPLSources defines the discovery queries.
type VBPLSources struct {
	SweepAgencies   []string `yaml:"sweep_agencies" json:"sweep_agencies"`
	KeywordAgencies []string `yaml:"keyword_agencies" json:"keyword_agencies"`
	Keywords        []string `yaml:"keywords" json:"keywords"`
}

// RelationCode defines a source relation.
type RelationCode struct {
	Kind        string `yaml:"kind" json:"kind"`
	Label       string `yaml:"label" json:"label"`
	Amending    bool   `yaml:"amending" json:"amending"`
	Superseding bool   `yaml:"superseding" json:"superseding"`
}

// StatusCode defines a source validity status.
type StatusCode struct {
	Class   string `yaml:"class" json:"class"`
	Current bool   `yaml:"current" json:"current"`
}

// Parse decodes a YAML or JSON source list.
func Parse(data []byte) (List, error) {
	var list List
	if err := yaml.Unmarshal(data, &list); err != nil {
		return List{}, fmt.Errorf("parse source list: %w", err)
	}
	return list, nil
}

// RelationCodesHash returns the hash of the canonical relation code JSON.
func (l List) RelationCodesHash() string {
	data, _ := json.Marshal(l.RelationCodes)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
