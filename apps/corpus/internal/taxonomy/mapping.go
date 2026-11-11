package taxonomy

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Mapping records package nodes and flattened structures.
type Mapping struct {
	PackageSHA256 string       `yaml:"package_sha256"`
	Nodes         []MappedNode `yaml:"nodes"`
	Flattened     []Flattened  `yaml:"flattened"`
}

// MappedNode records a stable package node.
type MappedNode struct {
	GUID       string `yaml:"guid"`
	Code       string `yaml:"code"`
	ParentCode string `yaml:"parent"`
	Level      int    `yaml:"level"`
	Kind       string `yaml:"kind"`
	Name       string `yaml:"name"`
	Ordinal    int    `yaml:"ordinal,omitempty"`
	Status     string `yaml:"status"`
}

// Flattened records a structural node folded into a coded ancestor.
type Flattened struct {
	GUID   string `yaml:"guid"`
	Into   string `yaml:"into"`
	Reason string `yaml:"reason"`
}

// ReadMapping parses a mapping file.
func ReadMapping(data []byte) (Mapping, error) {
	var m Mapping
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return m, fmt.Errorf("decode mapping: %w", err)
	}
	return m, nil
}

// Encode returns a deterministic YAML representation of the mapping.
func (m Mapping) Encode() []byte {
	m.Nodes = append([]MappedNode{}, m.Nodes...)
	m.Flattened = append([]Flattened{}, m.Flattened...)
	sort.Slice(m.Nodes, func(i, j int) bool { return codeLess(m.Nodes[i].Code, m.Nodes[j].Code) })
	sort.Slice(m.Flattened, func(i, j int) bool { return m.Flattened[i].GUID < m.Flattened[j].GUID })
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	_ = enc.Encode(m)
	_ = enc.Close()
	return b.Bytes()
}
func codeLess(a, b string) bool {
	x, y := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(x) && i < len(y); i++ {
		if x[i] == y[i] {
			continue
		}
		ax, bx := x[i], y[i]
		ap, bp := "", ""
		if ax[0] < '0' || ax[0] > '9' {
			ap = ax[:1]
			ax = ax[1:]
		}
		if bx[0] < '0' || bx[0] > '9' {
			bp = bx[:1]
			bx = bx[1:]
		}
		if ap != bp {
			return ap < bp
		}
		an, _ := strconv.Atoi(ax)
		bn, _ := strconv.Atoi(bx)
		if an != bn {
			return an < bn
		}
		return x[i] < y[i]
	}
	return len(x) < len(y)
}
