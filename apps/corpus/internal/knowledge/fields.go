package knowledge

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

var (
	idPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,63}$`)
	relPattern  = regexp.MustCompile(`^[0-9]{8}(_[0-9]{1,2})?$`)
	hashPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	intPattern  = regexp.MustCompile(`^[1-9][0-9]*$`)
	reasonCodes = []string{"false_positive", "expected", "source_defect", "out_of_scope", "deferred", "resolved"}
	reviewers   = []string{"owner", "coordinator", "primary"}
	effectKinds = []string{"repeals", "amends_supplements", "replaces", "suspends"}

	relationKinds = []string{"repeals", "amends_supplements", "replaces", "suspends", "implements", "guided_by", "legal_basis", "cites", "references"}
)

type parser struct {
	country  string
	problems []string
}

func (p *parser) add(format string, args ...any) {
	p.problems = append(p.problems, fmt.Sprintf(format, args...))
}

func (p *parser) fields(at string, n *yaml.Node, names ...string) map[string]*yaml.Node {
	values := map[string]*yaml.Node{}
	if n == nil {
		return values
	}
	if n.Kind != yaml.MappingNode {
		p.add("%s: must be a mapping", at)
		return values
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		name := n.Content[i].Value
		switch {
		case !slices.Contains(names, name):
			p.add("%s: unknown field %q", at, name)
		case values[name] != nil:
			p.add("%s: duplicate field %q", at, name)
		default:
			values[name] = n.Content[i+1]
		}
	}
	for _, name := range names {
		if values[name] == nil {
			p.add("%s: missing field %q", at, name)
		}
	}
	return values
}

func (p *parser) text(at string, n *yaml.Node) string {
	if n == nil {
		return ""
	}
	if n.Kind != yaml.ScalarNode || n.ShortTag() != "!!str" {
		p.add("%s: must be a string", at)
		return ""
	}
	return n.Value
}

func (p *parser) positive(at string, n *yaml.Node) int {
	if n == nil {
		return 0
	}
	var value int
	if n.Kind != yaml.ScalarNode || n.ShortTag() != "!!int" || !intPattern.MatchString(n.Value) || n.Decode(&value) != nil {
		p.add("%s: must be a positive decimal integer", at)
		return 0
	}
	return value
}

func (p *parser) matching(at string, n *yaml.Node, pattern *regexp.Regexp, form string) string {
	value := p.text(at, n)
	if n != nil && n.ShortTag() == "!!str" && !pattern.MatchString(value) {
		p.add("%s: must be %s", at, form)
	}
	return value
}

func (p *parser) hash(at string, n *yaml.Node) string {
	return p.matching(at, n, hashPattern, "sha256: and 64 lower-case hexadecimal characters")
}

func (p *parser) rel(at string, n *yaml.Node) string {
	return p.matching(at, n, relPattern, "a release name")
}

func (p *parser) slug(at string, n *yaml.Node) string {
	return p.matching(at, n, idPattern, "a slug")
}

func (p *parser) oneOf(at string, n *yaml.Node, allowed []string) string {
	value := p.text(at, n)
	if n != nil && n.ShortTag() == "!!str" && !slices.Contains(allowed, value) {
		p.add("%s: %q is not allowed", at, value)
	}
	return value
}

func (p *parser) length(at string, n *yaml.Node, low, high int) string {
	value := p.text(at, n)
	if count := utf8.RuneCountInString(value); n != nil && n.ShortTag() == "!!str" && (count < low || count > high) {
		p.add("%s: must have %d to %d characters", at, low, high)
	}
	return value
}

func (p *parser) date(at string, n *yaml.Node) string {
	value := p.text(at, n)
	if n != nil && n.ShortTag() == "!!str" {
		if _, err := time.Parse("2006-01-02", value); err != nil {
			p.add("%s: must be a date YYYY-MM-DD", at)
		}
	}
	return value
}

func (p *parser) key(at string, n *yaml.Node) string {
	value := p.text(at, n)
	prefix := p.country + ":"
	if n != nil && n.ShortTag() == "!!str" && (!strings.HasPrefix(value, prefix) || len(value) == len(prefix) || strings.ContainsFunc(value, unicode.IsSpace)) {
		p.add("%s: key must start with %s", at, prefix)
	}
	return value
}

func (p *parser) anchor(at string, n *yaml.Node) string {
	value := p.text(at, n)
	if n == nil || n.ShortTag() != "!!str" {
		return value
	}
	if strings.Contains(value, "~") {
		p.add("%s: a ~k component is refused", at)
	} else if !validAnchor(value) {
		p.add("%s: invalid anchor %q", at, value)
	}
	return value
}

func validAnchor(anchor string) bool {
	for _, component := range strings.Split(anchor, "/") {
		kind, token, found := strings.Cut(component, "-")
		if !found || kind == "" || token == "" || strings.ToLower(component) != component || strings.ContainsFunc(component, unicode.IsSpace) {
			return false
		}
		for _, c := range kind {
			if c < 'a' || c > 'z' {
				return false
			}
		}
	}
	return true
}
