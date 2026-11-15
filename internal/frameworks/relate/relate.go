package relate

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"danny.vn/trachuan/internal/frameworks/parse"
	"danny.vn/trachuan/internal/frameworks/registry"
)

var versionTokenPattern = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)*$`)

type Key struct {
	Code     string `json:"code"`
	Version  string `json:"version"`
	Citation string `json:"citation"`
}
type Evidence struct {
	Value    string   `json:"value"`
	Requires []string `json:"requires"`
}
type Edge struct {
	From     Key       `json:"from"`
	To       Key       `json:"to"`
	Relation string    `json:"relation"`
	Source   string    `json:"source"`
	Resolved bool      `json:"resolved"`
	Evidence *Evidence `json:"evidence,omitempty"`
}
type Input struct {
	From     Key
	To       Key
	Relation string
	Source   string
	Evidence string
}
type Stats struct{ Unresolved, Dropped, Duplicates int }

func Build(reg *registry.Registry, catalogs map[string]*parse.Result, inputs []Input) ([]Edge, Stats, error) {
	var stats Stats
	for _, v := range reg.Frameworks {
		result := catalogs[v.Code+":"+v.Version]
		if result == nil {
			continue
		}
		for _, n := range result.Nodes {
			stats.Dropped += n.DroppedMappings
			for _, m := range n.Mappings {
				source := "nist-oscal"
				if v.Parser == "nist-csf-2-xlsx" {
					source = "nist-csf-references"
					if m.Relation != "related" {
						source = "nist-csf-withdrawals"
					}
				}
				inputs = append(inputs, Input{From: Key{v.Code, v.Version, n.Citation}, To: Key{m.TargetFramework, m.TargetVersion, m.TargetCitation}, Relation: "related", Source: source})
			}
		}
	}
	var edges []Edge
	seen := map[string]bool{}
	for _, in := range inputs {
		code, version, exists := resolveVersion(reg, in.To.Code, in.To.Version)
		in.To.Code, in.To.Version = code, version
		in.From.Citation = lookupCitation(reg, in.From)
		in.To.Citation = lookupCitation(reg, in.To)
		if in.Relation != "related" && in.Relation != "equivalent" && in.Relation != "subset" && in.Relation != "superset" && in.Relation != "intersects" {
			return nil, stats, fmt.Errorf("invalid relation")
		}
		edge := Edge{From: in.From, To: in.To, Relation: in.Relation, Source: in.Source}
		if exists {
			if result := catalogs[code+":"+version]; result != nil {
				for _, n := range result.Nodes {
					if norm(n.Citation) == norm(in.To.Citation) {
						edge.To.Citation = n.Citation
						edge.Resolved = true
						break
					}
				}
			}
		}
		if edge.Resolved {
			sourceFound := false
			if result := catalogs[in.From.Code+":"+in.From.Version]; result != nil {
				for _, node := range result.Nodes {
					if norm(node.Citation) == norm(in.From.Citation) {
						edge.From.Citation = node.Citation
						sourceFound = true
						break
					}
				}
			}
			edge.Resolved = sourceFound
		}
		if !edge.Resolved {
			stats.Unresolved++
		}
		if in.Evidence != "" {
			requires := []string{}
			for _, key := range []Key{edge.From, edge.To} {
				found := false
				for _, v := range reg.Frameworks {
					if v.Code != key.Code || v.Version != key.Version {
						continue
					}
					if v.Access != "free" && v.Access != "licensed" {
						return nil, stats, fmt.Errorf("evidence access not classified")
					}
					found = true
					if v.Access == "licensed" {
						handle := "fw:" + v.Code + ":" + v.Version
						if len(requires) == 0 || requires[0] != handle {
							requires = append(requires, handle)
						}
					}
					break
				}
				if !found {
					return nil, stats, fmt.Errorf("evidence version not registered")
				}
			}

			sort.Strings(requires)
			edge.Evidence = &Evidence{in.Evidence, requires}
		}
		b, _ := json.Marshal([]any{edge.From, edge.To, edge.Relation, edge.Source})
		id := string(b)
		if seen[id] {
			stats.Duplicates++
			if !edge.Resolved {
				stats.Unresolved--
			}
			continue
		}
		seen[id] = true
		edges = append(edges, edge)
	}
	sort.Slice(edges, func(i, j int) bool {
		a, b := edges[i], edges[j]
		for _, pair := range [][2]string{{a.From.Code, b.From.Code}, {a.From.Version, b.From.Version}, {a.From.Citation, b.From.Citation}, {a.To.Code, b.To.Code}, {a.To.Version, b.To.Version}, {a.To.Citation, b.To.Citation}, {a.Relation, b.Relation}, {a.Source, b.Source}} {
			if pair[0] != pair[1] {
				return pair[0] < pair[1]
			}
		}
		return false
	})
	return edges, stats, nil
}
func lookupCitation(reg *registry.Registry, key Key) string {
	for _, v := range reg.Frameworks {
		if v.Code == key.Code && v.Version == key.Version && v.CitationScheme == "oscal-catalog" {
			if citation := nistCitation(key.Citation); citation != "" {
				return citation
			}
			break
		}
	}
	return key.Citation
}
func norm(s string) string { return strings.ToUpper(strings.ReplaceAll(s, " ", "")) }
func resolveVersion(reg *registry.Registry, name, version string) (string, string, bool) {
	original := name
	name = strings.ToLower(strings.Join(strings.Fields(name), " "))
	code := ""
	for _, v := range reg.Frameworks {
		if strings.EqualFold(v.Code, name) {
			code = v.Code
			break
		}
	}
	if code == "" {
		for _, v := range reg.Frameworks {
			for _, alias := range v.Aliases {
				if strings.ToLower(strings.Join(strings.Fields(alias), " ")) == name {
					code = v.Code
					break
				}
			}
		}
	}
	if code == "" && version == "" {
		longest := 0
		for _, v := range reg.Frameworks {
			for _, alias := range append([]string{v.Code}, v.Aliases...) {
				prefix := strings.ToLower(strings.Join(strings.Fields(alias), " "))
				if len(prefix) <= longest || !strings.HasPrefix(name, prefix) {
					continue
				}
				suffix := strings.TrimSpace(strings.TrimPrefix(name, prefix))
				if token := versionToken(suffix); token != "" {
					code = v.Code
					version = token
					longest = len(prefix)
				}
			}
		}
	}
	if code == "" {
		return original, version, false
	}
	if version != "" {
		token := versionToken(version)
		if token == "" {
			return code, version, false
		}
		major, _, _ := strings.Cut(token, ".")
		for _, wanted := range []string{token, major} {
			for _, v := range reg.Frameworks {
				if v.Code == code && versionToken(v.Version) == wanted {
					return code, v.Version, true
				}
			}
		}
		return code, version, false
	}
	var best *registry.Framework
	for i := range reg.Frameworks {
		v := &reg.Frameworks[i]
		if v.Code != code {
			continue
		}
		if best == nil || date(v) > date(best) || (date(v) == date(best) && v.Ordinal > best.Ordinal) {
			best = v
		}
	}
	if best == nil {
		return code, version, false
	}
	return code, best.Version, true
}
func versionToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, prefix := range []string{"revision", "rev", "v", "r"} {
		if strings.HasPrefix(value, prefix) {
			value = strings.TrimSpace(strings.TrimPrefix(value, prefix))
			break
		}
	}
	if !versionTokenPattern.MatchString(value) {
		return ""
	}
	return value
}
func date(v *registry.Framework) string {
	if v.Released == nil {
		return ""
	}
	return *v.Released
}
func Write(w io.Writer, edges []Edge) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, edge := range edges {
		if err := enc.Encode(edge); err != nil {
			return fmt.Errorf("write edge: %w", err)
		}
	}
	return nil
}
