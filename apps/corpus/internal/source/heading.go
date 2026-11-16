package source

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type HeadingExclusion struct {
	Key      string `yaml:"key" json:"key"`
	Anchor   string `yaml:"anchor" json:"anchor"`
	Reason   string `yaml:"reason" json:"reason"`
	Reviewed string `yaml:"reviewed" json:"reviewed"`
}

type HeadingReview struct {
	Release          string `yaml:"release" json:"release"`
	Projection       int    `yaml:"projection" json:"projection"`
	ScanSHA256       string `yaml:"scan_sha256" json:"scan_sha256"`
	ExclusionsSHA256 string `yaml:"exclusions_sha256" json:"exclusions_sha256"`
	Reviewer         string `yaml:"reviewer" json:"reviewer"`
	Date             string `yaml:"date" json:"date"`
}

func (review *HeadingReview) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("heading review must be an object")
	}
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i].Value, node.Content[i+1]
		switch key {
		case "projection":
			if value.Kind != yaml.ScalarNode || value.ShortTag() != "!!int" {
				return fmt.Errorf("heading projection must be an integer")
			}
		case "release", "scan_sha256", "exclusions_sha256", "reviewer", "date":
		default:
			return fmt.Errorf("unknown heading review field %q", key)
		}
	}
	type fields HeadingReview
	if err := node.Decode((*fields)(review)); err != nil {
		return fmt.Errorf("decode heading review: %w", err)
	}
	return nil
}

func decodeHeadingInput[T any](data []byte) ([]T, error) {
	if strings.TrimSpace(string(data)) == "" {
		return []T{}, nil
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var entries []T
	if err := decoder.Decode(&entries); err != nil {
		return nil, fmt.Errorf("decode heading input: %w", err)
	}
	if entries == nil {
		return nil, fmt.Errorf("heading input must be a sequence")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return nil, fmt.Errorf("decode trailing heading input: %w", err)
		}
		return nil, fmt.Errorf("multiple heading input documents")
	}
	return entries, nil
}

func ParseHeadingExclusions(data []byte) ([]HeadingExclusion, error) {
	entries, err := decodeHeadingInput[HeadingExclusion](data)
	if err != nil {
		return nil, err
	}
	seen := map[[2]string]bool{}
	for _, e := range entries {
		identity := [2]string{e.Key, e.Anchor}
		if !regexp.MustCompile(`^vn:(vbpl|chinhphu):[0-9]+$`).MatchString(e.Key) || strings.TrimSpace(e.Anchor) == "" || strings.TrimSpace(e.Reason) == "" || seen[identity] {
			return nil, fmt.Errorf("invalid or duplicate heading exclusion")
		}
		if _, err = time.Parse("2006-01-02", e.Reviewed); err != nil {
			return nil, fmt.Errorf("invalid heading exclusion date: %w", err)
		}
		seen[identity] = true
	}
	return entries, nil
}

func ParseHeadingReviews(data []byte) ([]HeadingReview, error) {
	entries, err := decodeHeadingInput[HeadingReview](data)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if !regexp.MustCompile(`^[0-9]{8}(_[0-9]{1,2})?$`).MatchString(e.Release) || e.Projection != 1 || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(e.ScanSHA256) || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(e.ExclusionsSHA256) || !regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`).MatchString(e.Reviewer) || seen[e.Release] {
			return nil, fmt.Errorf("invalid or duplicate heading review")
		}
		if _, err = time.Parse("2006-01-02", e.Date); err != nil {
			return nil, fmt.Errorf("invalid heading review date: %w", err)
		}
		seen[e.Release] = true
	}
	return entries, nil
}
