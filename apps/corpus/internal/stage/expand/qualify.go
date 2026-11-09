package expand

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/source"
)

// Target records one outward relation and the response that established it.
type Target struct {
	TargetKey      string `json:"target_key"`
	RawCode        string `json:"raw_code"`
	Kind           string `json:"kind"`
	Method         string `json:"method"`
	Response       string `json:"response"`
	ResponseSHA256 string `json:"response_sha256"`
}

// NumberOnly records a relation without an explicit target id.
type NumberOnly struct {
	RawCode string `json:"raw_code"`
	Number  string `json:"number"`
}

// Observation holds qualified outward relations for a seed.
type Observation struct {
	Version      int          `json:"version"`
	SeedKey      string       `json:"seed_key"`
	Targets      []Target     `json:"targets"`
	NumberOnly   []NumberOnly `json:"number_only"`
	UnknownCodes []string     `json:"unknown_codes"`
}

type identifier string

func (i *identifier) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}
	var s string
	if len(b) > 0 && b[0] == '"' {
		if err := json.Unmarshal(b, &s); err != nil {
			return fmt.Errorf("decode id: %w", err)
		}
	} else {
		var n json.Number
		if err := json.Unmarshal(b, &n); err != nil {
			return fmt.Errorf("decode id: %w", err)
		}
		s = n.String()
	}
	*i = identifier(strings.TrimSpace(s))
	return nil
}

// Qualify extracts only explicit outward targets allowed by the frozen code table.
func Qualify(seedKey string, detail, diagram []byte, codes map[string]source.RelationCode) (Observation, error) {
	var d struct {
		Data struct {
			References []struct {
				Code   identifier `json:"referenceType"`
				Target struct {
					ID     identifier `json:"id"`
					Number string     `json:"docNum"`
				} `json:"targetDocument"`
			} `json:"references"`
		} `json:"data"`
	}
	var g struct {
		Data struct {
			ByType map[string][]struct {
				ID   identifier `json:"id"`
				Name string     `json:"name"`
			} `json:"documentNamesByType"`
		} `json:"data"`
	}
	if err := json.Unmarshal(detail, &d); err != nil {
		return Observation{}, fmt.Errorf("decode detail: %w", err)
	}
	if err := json.Unmarshal(diagram, &g); err != nil {
		return Observation{}, fmt.Errorf("decode diagram: %w", err)
	}
	out := Observation{Version: 1, SeedKey: seedKey, Targets: []Target{}, NumberOnly: []NumberOnly{}, UnknownCodes: []string{}}
	unknown := map[string]bool{}
	validID := regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z_-]{0,63}$`)
	country, seed, ok := strings.Cut(seedKey, ":vbpl:")
	if !ok || country == "" || seed == "" {
		return Observation{}, fmt.Errorf("invalid seed key %q", seedKey)
	}
	prefix := country + ":vbpl:"
	seenTargets := map[[3]string]bool{}
	seenNumbers := map[NumberOnly]bool{}
	add := func(code, id, number, method, response string, body []byte) {
		c, ok := codes[code]
		if !ok {
			unknown[code] = true
		}
		id = strings.TrimSpace(id)
		if id == "" {
			if n := strings.TrimSpace(number); n != "" {
				entry := NumberOnly{code, n}
				if !seenNumbers[entry] {
					seenNumbers[entry] = true
					out.NumberOnly = append(out.NumberOnly, entry)
				}
			}
			return
		}
		if !ok {
			return
		}
		switch c.Kind {
		case "legal_basis", "implements", "guided_by", "amends_supplements", "replaces", "repeals", "suspends", "consolidates", "cites", "references":
		default:
			return
		}
		if !validID.MatchString(id) || strings.HasPrefix(strings.ToLower(id), "vbpqta_") || id == seed {
			return
		}
		key := prefix + id
		identity := [3]string{key, code, method}
		if seenTargets[identity] {
			return
		}
		seenTargets[identity] = true
		h := sha256.Sum256(body)
		out.Targets = append(out.Targets, Target{key, code, c.Kind, method, response, hex.EncodeToString(h[:])})
	}
	for _, ref := range d.Data.References {
		add(string(ref.Code), string(ref.Target.ID), ref.Target.Number, "vbpl_references", "detail.json", detail)
	}
	for code, docs := range g.Data.ByType {
		for _, doc := range docs {
			add(code, string(doc.ID), doc.Name, "vbpl_diagram", "diagram.json", diagram)
		}
	}
	for code := range unknown {
		out.UnknownCodes = append(out.UnknownCodes, code)
	}
	sort.Strings(out.UnknownCodes)
	sort.Slice(out.Targets, func(i, j int) bool {
		a, b := out.Targets[i], out.Targets[j]
		if a.TargetKey != b.TargetKey {
			return a.TargetKey < b.TargetKey
		}
		if a.RawCode != b.RawCode {
			return a.RawCode < b.RawCode
		}
		return a.Method < b.Method
	})
	sort.Slice(out.NumberOnly, func(i, j int) bool {
		a, b := out.NumberOnly[i], out.NumberOnly[j]
		if a.RawCode != b.RawCode {
			return a.RawCode < b.RawCode
		}
		return a.Number < b.Number
	})
	return out, nil
}
