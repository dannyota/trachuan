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
	partCount := 0
	found, last := 0, 0
	any := false
	childAmbiguous := false
	for _, child := range n.ordered {
		if err := ctx.Err(); err != nil {
			return err
		}
		if renderErr := tree.render(ctx, child); renderErr != nil {
			return renderErr
		}
		text := tree.nodes[child].text
		if text == "" {
			continue
		}
		partCount++
		match := findReadChild(raw, text, last)
		if match.count > 0 {
			any = true
		}
		if match.count == 1 && match.first >= last {
			found++
			last = match.first + len(text)
			if match.first < n.firstChild {
				n.firstChild = match.first
			}
		}
		childAmbiguous = childAmbiguous || tree.nodes[child].ambiguous
	}

	switch found {
	case partCount:
		n.ambiguous = childAmbiguous
	case 0:
		if any {
			n.ambiguous = true
			break
		}
		var out strings.Builder
		out.WriteString(raw)
		for _, child := range n.ordered {
			if err := ctx.Err(); err != nil {
				return err
			}
			text := tree.nodes[child].text
			if text == "" {
				continue
			}
			if out.Len() > 0 {
				out.WriteByte('\n')
			}
			out.WriteString(text)
			n.ambiguous = n.ambiguous || tree.nodes[child].ambiguous
		}
		n.text = out.String()

	default:
		n.ambiguous = true
	}
	if !n.ambiguous {
		var own strings.Builder
		last = 0
		for _, child := range n.children {
			if err := ctx.Err(); err != nil {
				return err
			}
			text := tree.nodes[child].text
			if text == "" {
				continue
			}
			at := strings.Index(raw, text)
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
