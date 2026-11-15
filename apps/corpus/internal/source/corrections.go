package source

import (
	"fmt"
	"io"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

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

type CorrectionSet struct {
	StatusClass *string `yaml:"status_class"`
	EffFrom     *string `yaml:"eff_from"`
	EffTo       *string `yaml:"eff_to"`
}

type CorrectionEvidence struct {
	Key    string `yaml:"key"`
	Anchor string `yaml:"anchor"`
	URL    string `yaml:"url"`
	Quote  string `yaml:"quote"`
}

func ParseCorrections(data []byte) ([]Correction, error) {
	entries, err := DecodeCorrections(data)
	if err != nil {
		return nil, err
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

func DecodeCorrections(data []byte) ([]Correction, error) {
	if strings.TrimSpace(string(data)) == "" {
		data = []byte("[]")
	}
	var entries []Correction
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&entries); err != nil {
		return nil, fmt.Errorf("parse corrections: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return nil, fmt.Errorf("parse trailing corrections: %w", err)
		}
		return nil, fmt.Errorf("multiple correction documents")
	}
	return entries, nil
}
