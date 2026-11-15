package parse

import (
	"fmt"
	"strings"
)

type Mapping struct {
	TargetFramework string
	TargetVersion   string
	TargetCitation  string
	Relation        string
	Unresolved      bool
}
type Attributes struct {
	AssetType            string
	SecurityFunction     string
	ImplementationGroups []string
}
type Node struct {
	Citation        string
	CitationNorm    string
	Kind            string
	Parent          int
	Ordinal         int
	Title           string
	PublisherTitle  string
	Body            string
	Status          string
	Mappings        []Mapping
	DroppedMappings int
	Attributes      Attributes
}
type Result struct {
	Nodes            []Node
	InvalidCitations int
	Duplicates       int
}

func Parse(name string, data []byte, code, version string, expected *int) (*Result, error) {
	var nodes []Node
	var err error
	switch name {
	case "nist-oscal-800-53":
		nodes, err = parseOSCAL(data, code, version)
	case "nist-csf-2-xlsx":
		nodes, err = parseCSF(data, code, version)
	case "cis-controls-8-xlsx":
		nodes, err = parseCIS(data)
	default:
		return nil, fmt.Errorf("unknown parser")
	}
	if err != nil && len(nodes) == 0 {
		return nil, err
	}
	result := &Result{Nodes: nodes}
	seen := map[string]bool{}
	for i := range nodes {
		n := &nodes[i]
		n.CitationNorm = strings.ToUpper(strings.ReplaceAll(n.Citation, " ", ""))
		if !validCitation(n.Citation) {
			result.InvalidCitations++
		}
		if seen[n.CitationNorm] {
			result.Duplicates++
		}
		seen[n.CitationNorm] = true
	}
	if err != nil {
		return result, err
	}
	if err = validateResult(result, expected); err != nil {
		return result, err
	}
	return result, nil
}
func validCitation(s string) bool {
	if len(s) < 1 || len(s) > 64 || strings.HasPrefix(s, " ") || strings.HasSuffix(s, " ") || strings.Contains(s, "  ") {
		return false
	}
	for _, c := range []byte(s) {
		if c < 0x20 || c > 0x7e {
			return false
		}
	}
	return true
}
func validate(nodes []Node, expected *int) error {
	r := &Result{Nodes: nodes}
	seen := map[string]bool{}
	for _, n := range nodes {
		norm := strings.ToUpper(strings.ReplaceAll(n.Citation, " ", ""))
		if !validCitation(n.Citation) {
			r.InvalidCitations++
		}
		if seen[norm] {
			r.Duplicates++
		}
		seen[norm] = true
	}
	return validateResult(r, expected)
}
func validateResult(r *Result, expected *int) error {
	if r.InvalidCitations > 0 || r.Duplicates > 0 {
		return fmt.Errorf("invalid or duplicate citations")
	}
	if expected == nil {
		return fmt.Errorf("expected control count unset")
	}
	if len(r.Nodes) != *expected {
		return fmt.Errorf("control count mismatch")
	}
	return nil
}
