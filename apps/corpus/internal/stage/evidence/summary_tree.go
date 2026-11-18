package evidence

import (
	"bytes"
	"cmp"
	"slices"
	"strings"
)

type summaryDocument struct {
	Key, NumberNorm, StatusClass string
	EffFrom, EffTo               *string
	IsCurrent, ValidityConflict  bool
	TextSource                   string
	TextComplete, Baseline       bool
}

type assignment struct {
	Node    int64  `json:"node"`
	Family  int64  `json:"family"`
	Method  string `json:"method"`
	Exclude bool   `json:"exclude"`
}

type summaryProvision struct {
	ID          int64
	ParentID    *int64
	Anchor      string
	TextSHA256  string
	Assignments []assignment
	Validity    []validityRow
}

type taxonomyNode struct {
	Code   string
	Served bool
}

type summaryRelation struct {
	Kind, Method    string
	SourceKey       string
	SourceNumber    string
	SourceInRelease bool
	SourceAnchor    *string
	TargetKey       *string
	TargetNumber    string
	TargetInRelease bool
	TargetAnchor    *string
	TargetNode      *string
}

func sortRowsByAnchor(rows []summaryProvision) {
	slices.SortFunc(rows, func(a, b summaryProvision) int { return strings.Compare(a.Anchor, b.Anchor) })
}

func (d summaryDocument) line() documentLine {
	return documentLine{Type: recordDocument, Key: d.Key, NumberNorm: d.NumberNorm, StatusClass: d.StatusClass, EffFrom: d.EffFrom, EffTo: d.EffTo, IsCurrent: d.IsCurrent, ValidityConflict: d.ValidityConflict, TextSource: d.TextSource, TextComplete: d.TextComplete, Baseline: d.Baseline}
}

type provisionTree struct {
	byID     map[int64]*summaryProvision
	suffixed map[string]bool
}

func cutSuffix(component string) (string, bool) {
	i := strings.LastIndexByte(component, '~')
	if i < 0 || i == len(component)-1 {
		return "", false
	}
	for _, c := range component[i+1:] {
		if c < '0' || c > '9' {
			return "", false
		}
	}
	return component[:i], true
}

func newProvisionTree(rows []summaryProvision) *provisionTree {
	t := &provisionTree{byID: make(map[int64]*summaryProvision, len(rows)), suffixed: map[string]bool{}}
	for i := range rows {
		p := &rows[i]
		t.byID[p.ID] = p
		prefix := ""
		for component := range strings.SplitSeq(p.Anchor, "/") {
			if base, ok := cutSuffix(component); ok {
				t.suffixed[prefix+base] = true
			}
			prefix += component + "/"
		}
	}
	return t
}

func (t *provisionTree) lineage(p *summaryProvision) ([]*summaryProvision, error) {
	chain := []*summaryProvision{p}
	for current := p; current.ParentID != nil; {
		parent := t.byID[*current.ParentID]
		if parent == nil {
			return nil, refuse("provision %s has no parent %d in its document", p.Anchor, *current.ParentID)
		}
		if len(chain) > len(t.byID) {
			return nil, refuse("provision %s has a parent cycle", p.Anchor)
		}
		chain = append(chain, parent)
		current = parent
	}
	return chain, nil
}

func (t *provisionTree) ambiguous(chain []*summaryProvision) bool {
	for _, p := range chain {
		prefix := ""
		for component := range strings.SplitSeq(p.Anchor, "/") {
			if _, suffixed := cutSuffix(component); suffixed || t.suffixed[prefix+component] {
				return true
			}
			prefix += component + "/"
		}
	}
	return false
}

func methodRank(method string) int {
	switch method {
	case "manual":
		return 0
	case "phapdien_seed":
		return 1
	case "inherited":
		return 2
	}
	return 3
}

func effectiveCodes(chain []*summaryProvision, nodes map[int64]taxonomyNode) ([]string, error) {
	decided := map[int64]bool{}
	codes := []string{}
	for _, p := range chain {
		best := map[int64]int{}
		for _, a := range p.Assignments {
			if rank, ok := best[a.Family]; !ok || methodRank(a.Method) < rank {
				best[a.Family] = methodRank(a.Method)
			}
		}
		excluded := map[int64]bool{}
		for _, a := range p.Assignments {
			if !decided[a.Family] && methodRank(a.Method) == best[a.Family] && a.Exclude {
				excluded[a.Family] = true
			}
		}
		for _, a := range p.Assignments {
			if decided[a.Family] || excluded[a.Family] || methodRank(a.Method) != best[a.Family] {
				continue
			}
			node, ok := nodes[a.Node]
			if !ok {
				return nil, refuse("taxonomy assignment names unknown node %d", a.Node)
			}
			if node.Served {
				codes = append(codes, node.Code)
			}
		}
		for family := range best {
			decided[family] = true
		}
	}
	slices.Sort(codes)
	return slices.Compact(codes), nil
}

func compareOptional(a, b *string) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return -1
	case b == nil:
		return 1
	}
	return strings.Compare(*a, *b)
}

func compareValidity(a, b validityRow) int {
	return cmp.Or(strings.Compare(a.StatusClass, b.StatusClass), compareOptional(a.EffFrom, b.EffFrom), compareOptional(a.EffTo, b.EffTo), strings.Compare(a.Method, b.Method))
}

func provisionLines(key string, rows []summaryProvision, nodes map[int64]taxonomyNode) ([]provisionLine, error) {
	for i := 1; i < len(rows); i++ {
		if rows[i-1].Anchor >= rows[i].Anchor {
			return nil, refuse("%s provisions are not in strict anchor order at %s", key, rows[i].Anchor)
		}
	}
	tree := newProvisionTree(rows)
	lines := make([]provisionLine, 0, len(rows))
	for i := range rows {
		p := &rows[i]
		chain, err := tree.lineage(p)
		if err != nil {
			return nil, err
		}
		codes, err := effectiveCodes(chain, nodes)
		if err != nil {
			return nil, err
		}
		validity := slices.Clone(p.Validity)
		if validity == nil {
			validity = []validityRow{}
		}
		slices.SortFunc(validity, compareValidity)
		line := provisionLine{Type: recordProvision, Key: key, Anchor: p.Anchor, Ambiguous: tree.ambiguous(chain), TextSHA256: p.TextSHA256, Nodes: codes, Validity: validity}
		if p.ParentID != nil {
			parent := tree.byID[*p.ParentID].Anchor
			line.Parent = &parent
		}
		lines = append(lines, line)
	}
	return lines, nil
}

func textMethod(method string) bool {
	text, reference := false, false
	for part := range strings.SplitSeq(method, ",") {
		switch part {
		case "text_rule/1", "reviewed":
			text = true
		case "vbpl_references":
			reference = true
		}
	}
	return text && !reference
}

func relationFor(r summaryRelation) relationIdentity {
	source := r.SourceKey
	out := relationIdentity{Type: recordRelation, Kind: r.Kind, Method: r.Method,
		Source: relationEnd{Key: &source, Number: r.SourceNumber, InRelease: r.SourceInRelease, Anchor: r.SourceAnchor},
		Target: relationEnd{Key: r.TargetKey, Number: r.TargetNumber, InRelease: r.TargetInRelease, Anchor: r.TargetAnchor}}
	text := textMethod(r.Method)
	if r.TargetAnchor != nil {
		exact := !text || (r.TargetNode != nil && *r.TargetNode == *r.TargetAnchor)
		out.Target.Exact = &exact
	}
	switch {
	case text && r.TargetNode != nil && (r.TargetAnchor == nil || *r.TargetNode != *r.TargetAnchor):
		out.Target.Named = r.TargetNode
	case !text && r.TargetAnchor == nil && r.TargetNode != nil:
		out.Target.Node = r.TargetNode
	}
	return out
}

type sortedRelation struct {
	identity []byte
	line     relationLine
}

func relationLines(rows []summaryRelation) ([]sortedRelation, error) {
	out := make([]sortedRelation, 0, len(rows))
	for _, r := range rows {
		identity := relationFor(r)
		data, err := encodeLine(identity)
		if err != nil {
			return nil, err
		}
		out = append(out, sortedRelation{identity: data, line: relationLine{relationIdentity: identity, Count: 1}})
	}
	slices.SortFunc(out, func(a, b sortedRelation) int { return bytes.Compare(a.identity, b.identity) })
	merged := out[:0]
	for _, r := range out {
		if n := len(merged); n > 0 && bytes.Equal(merged[n-1].identity, r.identity) {
			merged[n-1].line.Count++
			continue
		}
		merged = append(merged, r)
	}
	return merged, nil
}
