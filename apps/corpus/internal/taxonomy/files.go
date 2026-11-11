package taxonomy

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// CustomNode defines a manually added taxonomy node.
type CustomNode struct {
	Code        string `yaml:"code"`
	ParentCode  string `yaml:"parent"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// Override defines a manual article classification.
type Override struct {
	DocumentKey string `yaml:"document_key"`
	Anchor      string `yaml:"anchor"`
	Code        string `yaml:"code"`
	Exclude     bool   `yaml:"exclude"`
	Reason      string `yaml:"reason"`
}

// ReadCustom parses custom nodes.
func ReadCustom(data []byte) ([]CustomNode, error) {
	var v []CustomNode
	if err := decodeList(data, &v); err != nil {
		return nil, fmt.Errorf("custom: %w", err)
	}
	return v, nil
}

// ReadOverrides parses manual classifications.
func ReadOverrides(data []byte) ([]Override, error) {
	var v []Override
	if err := decodeList(data, &v); err != nil {
		return nil, fmt.Errorf("overrides: %w", err)
	}
	return v, nil
}
func decodeList(data []byte, dst any) error {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return err
	}
	if len(root.Content) != 1 || root.Content[0].Kind != yaml.SequenceNode {
		return fmt.Errorf("expected a YAML list")
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	return dec.Decode(dst)
}

// Validate checks relationships among mapping, custom nodes and overrides.
func Validate(m Mapping, custom []CustomNode, overrides []Override) error {
	codes := map[string]int{}
	activeGUIDs := map[string]bool{}
	for _, n := range m.Nodes {
		if n.GUID == "" || n.Code == "" || (n.Status == "active" && activeGUIDs[n.GUID]) || codes[n.Code] != 0 || n.Level < 1 || n.Level > 4 || (n.Status != "active" && n.Status != "retired") {
			return fmt.Errorf("invalid mapping node %q", n.GUID)
		}
		if n.Status == "active" {
			activeGUIDs[n.GUID] = true
		}
		codes[n.Code] = n.Level
	}
	ordinals := map[string]map[int]bool{}
	for _, n := range m.Nodes {
		if n.Level > 1 && (codes[n.ParentCode] == 0 || (n.Status == "active" && codes[n.ParentCode] != n.Level-1)) {
			return fmt.Errorf("invalid parent for %s", n.Code)
		}
		if n.Status == "active" && n.Ordinal < 1 || n.Status == "retired" && n.Ordinal != 0 {
			return fmt.Errorf("invalid ordinal for %s", n.Code)
		}
		if n.Status == "active" {
			if ordinals[n.ParentCode] == nil {
				ordinals[n.ParentCode] = map[int]bool{}
			}
			if ordinals[n.ParentCode][n.Ordinal] {
				return fmt.Errorf("duplicate ordinal for %s", n.Code)
			}
			ordinals[n.ParentCode][n.Ordinal] = true
		}
	}
	for parent, siblings := range ordinals {
		for i := 1; i <= len(siblings); i++ {
			if !siblings[i] {
				return fmt.Errorf("ordinal gap under %s", parent)
			}
		}
	}
	flatGUIDs := map[string]bool{}
	for _, f := range m.Flattened {
		if f.GUID == "" || activeGUIDs[f.GUID] || flatGUIDs[f.GUID] || codes[f.Into] == 0 || f.Reason == "" {
			return fmt.Errorf("invalid flattened node %q", f.GUID)
		}
		flatGUIDs[f.GUID] = true
	}
	pending := append([]CustomNode(nil), custom...)
	re := regexp.MustCompile(`^x[1-9][0-9]{0,3}$`)
	for len(pending) > 0 {
		next := []CustomNode{}
		for _, n := range pending {
			if codes[n.Code] != 0 {
				return fmt.Errorf("duplicate custom code %s", n.Code)
			}
			level := codes[n.ParentCode] + 1
			if level == 1 {
				next = append(next, n)
				continue
			}
			suffix := strings.TrimPrefix(n.Code, n.ParentCode+".")
			desc := strings.TrimSpace(n.Description)
			if level < 3 || level > 4 || suffix == n.Code || !re.MatchString(suffix) || strings.TrimSpace(n.Name) == "" || desc == "" || strings.Count(desc, ".")+strings.Count(desc, "!")+strings.Count(desc, "?") != 1 || !strings.ContainsAny(desc[len(desc)-1:], ".!?") {
				return fmt.Errorf("invalid custom node %s", n.Code)
			}
			codes[n.Code] = level
		}
		if len(next) == len(pending) {
			return fmt.Errorf("custom parent missing")
		}
		pending = next
	}
	seen := map[string]bool{}
	for _, o := range overrides {
		key := o.DocumentKey + "\x00" + o.Anchor + "\x00" + o.Code
		if strings.TrimSpace(o.DocumentKey) == "" || strings.TrimSpace(o.Anchor) == "" || strings.TrimSpace(o.Reason) == "" || codes[o.Code] == 0 || seen[key] {
			return fmt.Errorf("invalid override %s", o.Code)
		}
		seen[key] = true
	}
	return nil
}
