// Package source parses frozen source lists.
package source

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// List holds the sources and code tables for a country.
type List struct {
	Country       string                  `yaml:"country" json:"country"`
	Chinhphu      ChinhphuSources         `yaml:"chinhphu" json:"chinhphu,omitempty"`
	VBPL          VBPLSources             `yaml:"vbpl" json:"vbpl"`
	RelationCodes map[string]RelationCode `yaml:"relation_codes" json:"relation_codes"`
	StatusCodes   map[string]StatusCode   `yaml:"status_codes" json:"status_codes"`
}

// VBPLSources defines the discovery queries.
type VBPLSources struct {
	SelectionVersion int      `yaml:"selection_version" json:"selection_version,omitempty"`
	NamedNumbers     []string `yaml:"named_numbers" json:"named_numbers,omitempty"`
	SweepAgencies    []string `yaml:"sweep_agencies" json:"sweep_agencies"`
	KeywordAgencies  []string `yaml:"keyword_agencies" json:"keyword_agencies"`
	Keywords         []string `yaml:"keywords" json:"keywords"`
}

// ChinhphuSources defines secondary discovery queries.
type ChinhphuSources struct {
	Keywords     []string `yaml:"keywords" json:"keywords,omitempty"`
	NamedNumbers []string `yaml:"named_numbers" json:"named_numbers,omitempty"`
}

// NamedNumber is a number and its optional civil issue date.
type NamedNumber struct {
	Number string
	Date   string
}

// ParseNamedNumber parses a named entry and validates its date qualifier.
func ParseNamedNumber(entry string) (NamedNumber, error) {
	number, date, qualified := strings.Cut(entry, "@")
	if number == "" {
		return NamedNumber{}, fmt.Errorf("named entry %q: empty number", entry)
	}
	if qualified {
		if _, err := time.Parse("2006-01-02", date); err != nil {
			return NamedNumber{}, fmt.Errorf("named entry %q: invalid date: %w", entry, err)
		}
	} else {
		hasYear := false
		parts := strings.Split(number, "/")
		for i, part := range parts {
			if i > 0 && i < len(parts)-1 && len(part) == 4 && strings.Trim(part, "0123456789") == "" {
				hasYear = true
				break
			}
		}
		if !hasYear {
			return NamedNumber{}, fmt.Errorf("named entry %q: date required", entry)
		}
	}
	return NamedNumber{Number: number, Date: date}, nil
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
	for _, entries := range [][]string{list.VBPL.NamedNumbers, list.Chinhphu.NamedNumbers} {
		for _, entry := range entries {
			if _, err := ParseNamedNumber(entry); err != nil {
				return List{}, fmt.Errorf("parse source list: %w", err)
			}
		}
	}
	return list, nil
}

// RelationCodesHash returns the hash of the canonical relation code JSON.
func (l List) RelationCodesHash() string {
	data, _ := json.Marshal(l.RelationCodes)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
