package retrieval

import (
	"context"
	"strings"
)

type readTreeNode struct {
	provision     Provision
	legalCitation string
	citation      string
	children      []int
	ordered       []int
	text          string
	own           string
	firstChild    int
	ambiguous     bool
	rendered      bool
}

type readTree struct {
	nodes []readTreeNode
	byID  map[int64]int
}

func newReadTree(ctx context.Context, nodes []Provision) (*readTree, error) {
	tree := &readTree{nodes: make([]readTreeNode, len(nodes)), byID: make(map[int64]int, len(nodes))}
	for i, p := range nodes {
		if i%128 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		tree.nodes[i].provision = p
		tree.byID[p.ID] = i
	}
	for i, p := range nodes {
		if i%128 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		prefix := ""
		if p.ParentID != nil {
			if parent, ok := tree.byID[*p.ParentID]; ok && parent < i {
				tree.nodes[parent].children = append(tree.nodes[parent].children, i)
				prefix = tree.nodes[parent].legalCitation
			}
		}
		label := strings.TrimSpace(p.Label)
		if label != "" && (p.Kind == "dieu" || p.Kind == "khoan" || p.Kind == "diem") {
			if prefix != "" {
				prefix += ", "
			}
			prefix += label
		}
		tree.nodes[i].legalCitation = prefix
		if prefix == "" {
			tree.nodes[i].citation = label
		} else {
			tree.nodes[i].citation = prefix
		}
	}
	return tree, nil
}

func (tree *readTree) ancestry(ctx context.Context, index int) ([]Provision, error) {
	var chain []Provision
	seen := make(map[int64]bool)
	for {
		if len(chain)%128 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		p := tree.nodes[index].provision
		if seen[p.ID] {
			break
		}
		seen[p.ID] = true
		chain = append(chain, p)
		if p.ParentID == nil {
			break
		}
		parent, ok := tree.byID[*p.ParentID]
		if !ok {
			break
		}
		index = parent
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, nil
}

func (tree *readTree) render(ctx context.Context, index int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	n := &tree.nodes[index]
	if n.rendered {
		return nil
	}
	raw := strings.TrimSpace(n.provision.Text)
	n.text = raw
	n.own = raw
	n.firstChild = len(raw)
	if len(n.children) == 0 {
		n.rendered = true
		return nil
	}
	ordered, err := tree.orderChildren(ctx, n.children)
	if err != nil {
		return err
	}
	n.ordered = ordered
	var parts []int
	for _, child := range n.ordered {
		if renderErr := tree.render(ctx, child); renderErr != nil {
			return renderErr
		}
		if tree.nodes[child].text != "" {
			parts = append(parts, child)
		}
	}
	matches, err := tree.matchChildren(ctx, raw, parts)
	if err != nil {
		return err
	}
	found, last := 0, 0
	for i, child := range parts {
		if i%128 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		match := matches[child]
		if match.count == 1 && match.first >= last {
			found++
			last = match.first + len(tree.nodes[child].text)
		}
	}
	switch {
	case found == len(parts):
		for i, child := range parts {
			if i%128 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			at := matches[child].first
			if at < n.firstChild {
				n.firstChild = at
			}
			n.ambiguous = n.ambiguous || tree.nodes[child].ambiguous
		}
	case found == 0:
		any := false
		for i, child := range parts {
			if i%128 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			if matches[child].count > 0 {
				any = true
				break
			}
		}
		if any {
			n.ambiguous = true
			break
		}
		var out strings.Builder
		out.WriteString(raw)
		for i, child := range parts {
			if i%128 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			if out.Len() > 0 {
				out.WriteByte('\n')
			}
			out.WriteString(tree.nodes[child].text)
			n.ambiguous = n.ambiguous || tree.nodes[child].ambiguous
		}
		n.text = out.String()
	default:
		n.ambiguous = true
	}
	if !n.ambiguous {
		var own strings.Builder
		last = 0
		for i, child := range n.children {
			if i%128 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			text := tree.nodes[child].text
			if text == "" {
				continue
			}
			at := matches[child].first
			if at < 0 {
				continue
			}
			own.WriteString(raw[last:at])
			last = at + len(text)
		}
		own.WriteString(raw[last:])
		n.own = strings.TrimSpace(own.String())
	}
	n.rendered = true
	return nil
}

func (tree *readTree) leads(ctx context.Context, index int) ([]Unit, []Gap, error) {
	chain, err := tree.ancestry(ctx, index)
	if err != nil {
		return nil, nil, err
	}
	var units []Unit
	var gaps []Gap
	for i, p := range chain[:len(chain)-1] {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if p.Kind != "dieu" && p.Kind != "khoan" && p.Kind != "diem" {
			continue
		}
		ancestor := tree.byID[p.ID]
		if err := tree.render(ctx, ancestor); err != nil {
			return nil, nil, err
		}
		n := &tree.nodes[ancestor]
		if n.ambiguous {
			gaps = append(gaps, readAmbiguity(p))
			continue
		}
		raw := strings.TrimSpace(p.Text)
		lead := strings.TrimSpace(raw[:n.firstChild])
		if lead != "" && !readLabelOnly(lead, p.Label) {
			units = append(units, readTextUnit("lead", p, chain[:i+1], lead))
		}
	}
	return units, gaps, nil
}

func (tree *readTree) matchChildren(ctx context.Context, raw string, parts []int) (map[int]readMatch, error) {
	matches := make(map[int]readMatch, len(parts))
	if len(parts) <= 8 {
		for i, child := range parts {
			if i%128 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			text := tree.nodes[child].text
			matches[child] = readMatch{count: min(strings.Count(raw, text), 2), first: strings.Index(raw, text)}
		}
		return matches, ctx.Err()
	}
	matcher := newReadMatcher()
	ids := make(map[int]int, len(parts))
	for i, child := range parts {
		if i%128 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		text := tree.nodes[child].text
		if len(text) > len(raw) {
			matches[child] = readMatch{first: -1}
			continue
		}
		id, err := matcher.add(ctx, text)
		if err != nil {
			return nil, err
		}
		ids[child] = id
	}
	if err := matcher.scan(ctx, raw); err != nil {
		return nil, err
	}
	i := 0
	for child, id := range ids {
		if i%128 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		i++
		matches[child] = matcher.matches[id]
	}
	return matches, nil
}
