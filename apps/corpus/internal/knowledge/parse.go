package knowledge

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	countryPattern = regexp.MustCompile(`^[a-z]{2}$`)
	lineBreak      = regexp.MustCompile("\r\n|[\n\r\u0085\u2028\u2029]")
)

func SourcePath(country string) (string, error) {
	if !countryPattern.MatchString(country) {
		return "", fmt.Errorf("invalid country %q", country)
	}
	return "sources/" + country + "-knowledge.yaml", nil
}

func ParseFile(country, path string) (File, []byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return File{}, nil, fmt.Errorf("read knowledge %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
	if err != nil {
		return File{}, nil, fmt.Errorf("read knowledge %q: %w", path, err)
	}
	file, err := Parse(country, data)
	if err != nil {
		return File{}, nil, err
	}
	return file, data, nil
}

func Parse(country string, data []byte) (File, error) {
	if !countryPattern.MatchString(country) {
		return File{}, fmt.Errorf("invalid country %q", country)
	}
	if len(data) > MaxFileBytes {
		return File{}, invalid("file exceeds 8 MiB")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		if errors.Is(err, io.EOF) {
			return File{}, nil
		}
		return File{}, invalid("decode: " + err.Error())
	}
	var further yaml.Node
	if err := decoder.Decode(&further); !errors.Is(err, io.EOF) {
		return File{}, invalid("further documents are refused")
	}
	p := &parser{country: country}
	p.refuseGraph(&document, sourceLines(data))
	if len(p.problems) > 0 {
		return File{}, &ValidationError{Problems: p.problems}
	}
	root := document.Content[0]
	if root.Kind != yaml.SequenceNode {
		return File{}, invalid("the file must be one list")
	}
	if len(root.Content) > MaxEntries {
		return File{}, invalid(fmt.Sprintf("file has more than %d entries", MaxEntries))
	}
	file := File{}
	rules := newStructure(p)
	for position, node := range root.Content {
		p.entry(&file, rules, position, node)
	}
	if len(p.problems) > 0 {
		return File{}, &ValidationError{Problems: p.problems}
	}
	return file, nil
}

func invalid(problem string) error { return &ValidationError{Problems: []string{problem}} }

func sourceLines(data []byte) [][]rune {
	parts := lineBreak.Split(strings.TrimPrefix(string(data), "\uFEFF"), -1)
	lines := make([][]rune, len(parts))
	for i, part := range parts {
		lines[i] = []rune(part)
	}
	return lines
}

// yaml.v3 leaves TaggedStyle unset for the non-specific tag "!", but the node position still points at it.
func explicitTag(n *yaml.Node, lines [][]rune) bool {
	if n.Style&yaml.TaggedStyle != 0 {
		return true
	}
	if n.Line < 1 || n.Line > len(lines) {
		return false
	}
	line := lines[n.Line-1]
	return n.Column >= 1 && n.Column <= len(line) && line[n.Column-1] == '!'
}

func (p *parser) refuseGraph(n *yaml.Node, lines [][]rune) {
	if n.Kind == yaml.AliasNode {
		p.add("line %d: aliases are refused", n.Line)
		return
	}
	if n.Anchor != "" {
		p.add("line %d: anchors are refused", n.Line)
	}
	if explicitTag(n, lines) {
		p.add("line %d: explicit tags are refused", n.Line)
	}
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].ShortTag() == "!!merge" {
				p.add("line %d: merge keys are refused", n.Content[i].Line)
			}
		}
	}
	for _, child := range n.Content {
		p.refuseGraph(child, lines)
	}
}

func (p *parser) entry(file *File, rules *structure, position int, node *yaml.Node) {
	at := fmt.Sprintf("entry %d", position)
	if node.Kind != yaml.MappingNode {
		p.add("%s: must be a mapping", at)
		return
	}
	id, kind := "", peek(node, "kind")
	before := len(p.problems)
	if n := lookup(node, "id"); n != nil {
		if n.Value != "" {
			at = n.Value
		}
		if n.ShortTag() == "!!str" {
			id = n.Value
			rules.id(at, id)
		}
	}
	reviewed := func(m map[string]*yaml.Node) Reviewed { return p.reviewed(at+".reviewed", m["reviewed"]) }
	switch kind {
	case KindDecision:
		m := p.fields(at, node, "id", "kind", "target", "claim", "depends", "reviewed")
		p.text(at+".id", m["id"])
		d := p.decision(at, m)
		d.ID, d.Position, d.Reviewed = id, position, reviewed(m)
		if len(p.problems) == before {
			rules.decision(at, d)
			file.Decisions = append(file.Decisions, d)
		}
	case KindEffect, KindRelation:
		m := p.fields(at, node, "id", "kind", "target", "claim", "evidence", "depends", "reviewed")
		p.text(at+".id", m["id"])
		var c Claim
		if kind == KindEffect {
			c = p.effect(at, m)
		} else {
			c = p.relation(at, m)
		}
		c.ID, c.Kind, c.Position, c.Reviewed = id, kind, position, reviewed(m)
		if len(p.problems) == before {
			rules.claim(at, c)
			file.Claims = append(file.Claims, c)
		}
	default:
		p.add("%s: unknown kind %q", at, kind)
	}
}

func lookup(mapping *yaml.Node, name string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == name && mapping.Content[i+1].Kind == yaml.ScalarNode {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func peek(mapping *yaml.Node, name string) string {
	if n := lookup(mapping, name); n != nil && n.ShortTag() == "!!str" {
		return n.Value
	}
	return ""
}

func (p *parser) decision(at string, m map[string]*yaml.Node) ReviewDecision {
	target := p.fields(at+".target", m["target"], "finding")
	claim := p.fields(at+".claim", m["claim"], "reason_code", "reason")
	depends := p.fields(at+".depends", m["depends"], "fingerprint", "semantics", "first_rel")
	d := ReviewDecision{
		Finding:     p.hash(at+".target.finding", target["finding"]),
		ReasonCode:  p.oneOf(at+".claim.reason_code", claim["reason_code"], reasonCodes),
		Reason:      p.length(at+".claim.reason", claim["reason"], 1, 200),
		Fingerprint: p.hash(at+".depends.fingerprint", depends["fingerprint"]),
		Semantics:   p.positive(at+".depends.semantics", depends["semantics"]),
		FirstRel:    p.rel(at+".depends.first_rel", depends["first_rel"]),
	}
	return d
}

func (p *parser) effect(at string, m map[string]*yaml.Node) Claim {
	target := p.fields(at+".target", m["target"], "key", "anchor")
	claim := p.fields(at+".claim", m["claim"], "acting", "relation_kind")
	acting := p.fields(at+".claim.acting", claim["acting"], "key", "anchor")
	evidence := p.fields(at+".evidence", m["evidence"], "quote")
	depends := p.fields(at+".depends", m["depends"], "contract", "acting", "target")
	c := Claim{
		ActingKey:     p.key(at+".claim.acting.key", acting["key"]),
		ActingAnchor:  p.anchor(at+".claim.acting.anchor", acting["anchor"]),
		TargetKey:     p.key(at+".target.key", target["key"]),
		TargetAnchor:  p.anchor(at+".target.anchor", target["anchor"]),
		RelationKind:  p.oneOf(at+".claim.relation_kind", claim["relation_kind"], effectKinds),
		Quote:         p.length(at+".evidence.quote", evidence["quote"], 10, 200),
		Contract:      p.positive(at+".depends.contract", depends["contract"]),
		ActingDepends: p.provision(at+".depends.acting", depends["acting"]),
		TargetDepends: p.provision(at+".depends.target", depends["target"]),
	}
	c.TargetNumberNorm = c.TargetDepends.NumberNorm
	if c.ActingKey == c.TargetKey {
		p.add("%s: acting and target documents must differ", at)
	}
	return c
}

func (p *parser) relation(at string, m map[string]*yaml.Node) Claim {
	target := p.fields(at+".target", m["target"], "key")
	claim := p.fields(at+".claim", m["claim"], "acting", "relation_kind")
	acting := p.fields(at+".claim.acting", claim["acting"], "key")
	evidence := p.fields(at+".evidence", m["evidence"], "anchor", "quote")
	depends := p.fields(at+".depends", m["depends"], "contract", "provision", "target_number_norm")
	c := Claim{
		ActingKey:        p.key(at+".claim.acting.key", acting["key"]),
		ActingAnchor:     p.anchor(at+".evidence.anchor", evidence["anchor"]),
		TargetKey:        p.key(at+".target.key", target["key"]),
		RelationKind:     p.oneOf(at+".claim.relation_kind", claim["relation_kind"], relationKinds),
		Quote:            p.length(at+".evidence.quote", evidence["quote"], 10, 200),
		Contract:         p.positive(at+".depends.contract", depends["contract"]),
		ActingDepends:    p.provision(at+".depends.provision", depends["provision"]),
		TargetNumberNorm: p.text(at+".depends.target_number_norm", depends["target_number_norm"]),
	}
	if c.ActingKey == c.TargetKey {
		p.add("%s: acting and target documents must differ", at)
	}
	return c
}

func (p *parser) provision(at string, n *yaml.Node) ProvisionDepends {
	m := p.fields(at, n, "number_norm", "text_source", "text_sha256", "heading_sha256", "ancestors")
	d := ProvisionDepends{
		NumberNorm:    p.text(at+".number_norm", m["number_norm"]),
		TextSource:    p.text(at+".text_source", m["text_source"]),
		TextSHA256:    p.hash(at+".text_sha256", m["text_sha256"]),
		HeadingSHA256: p.hash(at+".heading_sha256", m["heading_sha256"]),
		Ancestors:     []Ancestor{},
	}
	list := m["ancestors"]
	if list == nil {
		return d
	}
	if list.Kind != yaml.SequenceNode {
		p.add("%s.ancestors: must be a list", at)
		return d
	}
	for i, node := range list.Content {
		item := fmt.Sprintf("%s.ancestors[%d]", at, i)
		fields := p.fields(item, node, "anchor", "heading_sha256")
		d.Ancestors = append(d.Ancestors, Ancestor{Anchor: p.anchor(item+".anchor", fields["anchor"]), HeadingSHA256: p.hash(item+".heading_sha256", fields["heading_sha256"])})
	}
	return d
}

func (p *parser) reviewed(at string, n *yaml.Node) Reviewed {
	if n == nil {
		return Reviewed{}
	}
	if n.Kind == yaml.MappingNode && peek(n, "method") == "model-adjudicated" {
		m := p.fields(at, n, "date", "by", "method", "batch", "sample_read", "sample_of")
		r := Reviewed{Date: p.date(at+".date", m["date"]), By: p.oneOf(at+".by", m["by"], reviewers), Method: "model-adjudicated", Batch: p.slug(at+".batch", m["batch"]), SampleRead: p.positive(at+".sample_read", m["sample_read"]), SampleOf: p.positive(at+".sample_of", m["sample_of"])}
		if r.SampleRead > r.SampleOf || r.SampleRead < r.SampleOf/10 || (r.SampleRead == r.SampleOf/10 && r.SampleOf%10 != 0) {
			p.add("%s: sample_read must be at least a tenth of sample_of and at most sample_of", at)
		}
		return r
	}
	m := p.fields(at, n, "date", "by", "method")
	return Reviewed{Date: p.date(at+".date", m["date"]), By: p.oneOf(at+".by", m["by"], reviewers), Method: p.oneOf(at+".method", m["method"], []string{"human"})}
}
