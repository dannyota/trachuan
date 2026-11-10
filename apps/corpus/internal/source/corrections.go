package source

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Correction is a reviewed override of a document's validity.
type Correction struct {
	Key      string             `yaml:"key"`
	Number   string             `yaml:"number"`
	Set      CorrectionSet      `yaml:"set"`
	Eligible *bool              `yaml:"eligible"`
	Hold     bool               `yaml:"hold"`
	Reason   string             `yaml:"reason"`
	Evidence CorrectionEvidence `yaml:"evidence"`
	Reviewed string             `yaml:"reviewed"`
}

// CorrectionSet contains the fields a correction may replace.
type CorrectionSet struct {
	StatusClass *string `yaml:"status_class"`
	EffFrom     *string `yaml:"eff_from"`
	EffTo       *string `yaml:"eff_to"`
}

// CorrectionEvidence identifies the text reviewed for a correction.
type CorrectionEvidence struct {
	Anchor string `yaml:"anchor"`
	Quote  string `yaml:"quote"`
}

// ParseCorrections decodes a correction list and validates its date syntax.
func ParseCorrections(data []byte) ([]Correction, error) {
	var entries []Correction
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&entries); err != nil {
		return nil, fmt.Errorf("parse corrections: %w", err)
	}
	for _, entry := range entries {
		for _, value := range []*string{entry.Set.EffFrom, entry.Set.EffTo} {
			if value != nil {
				if _, err := time.Parse("2006-01-02", *value); err != nil {
					return nil, fmt.Errorf("parse correction date: %w", err)
				}
			}
		}
	}
	return entries, nil
}
