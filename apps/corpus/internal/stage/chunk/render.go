package chunk

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"danny.vn/trachuan/apps/corpus/internal/stage"
)

type sourcePos struct {
	id     string
	offset int
}
type mappedRune struct {
	value     rune
	positions []sourcePos
}
type textRun []mappedRune

type treeNode struct {
	node         Node
	parent       *treeNode
	children     []*treeNode
	rendered     textRun
	own          []textRun
	ambiguous    bool
	hasAmbiguous bool
}

func tree(nodes []Node) ([]*treeNode, error) {
	byID := make(map[string]*treeNode, len(nodes))
	for _, n := range nodes {
		if n.NodeID == "" || byID[n.NodeID] != nil {
			return nil, stage.SourceData(fmt.Errorf("repeated or empty node id %q", n.NodeID))
		}
		byID[n.NodeID] = &treeNode{node: n}
	}
	var roots []*treeNode
	for _, n := range nodes {
		current := byID[n.NodeID]
		if n.ParentNodeID == nil {
			roots = append(roots, current)
			continue
		}
		parent := byID[*n.ParentNodeID]
		if parent == nil {
			return nil, stage.SourceData(fmt.Errorf("missing parent for %q", n.NodeID))
		}
		current.parent = parent
		parent.children = append(parent.children, current)
	}
	order := func(ns []*treeNode) {
		sort.Slice(ns, func(i, j int) bool {
			if ns[i].node.Ordinal != ns[j].node.Ordinal {
				return ns[i].node.Ordinal < ns[j].node.Ordinal
			}
			return ns[i].node.NodeID < ns[j].node.NodeID
		})
	}
	order(roots)
	for _, n := range byID {
		order(n.children)
	}
	seen := make(map[*treeNode]bool)
	var visit func(*treeNode)
	visit = func(n *treeNode) {
		if seen[n] {
			return
		}
		seen[n] = true
		for _, child := range n.children {
			visit(child)
		}
	}
	for _, root := range roots {
		visit(root)
	}
	if len(seen) != len(nodes) {
		return nil, stage.SourceData(fmt.Errorf("cyclic ancestry"))
	}
	return roots, nil
}

func sourceText(n Node) textRun {
	runes := []rune(n.Text)
	start, end := 0, len(runes)
	for start < end && unicode.IsSpace(runes[start]) {
		start++
	}
	for end > start && unicode.IsSpace(runes[end-1]) {
		end--
	}
	out := make(textRun, 0, end-start)
	for i := start; i < end; i++ {
		out = append(out, mappedRune{runes[i], []sourcePos{{n.NodeID, i}}})
	}
	return out
}

func plain(t textRun) string {
	var b strings.Builder
	for _, r := range t {
		b.WriteRune(r.value)
	}
	return b.String()
}
func join(a, b textRun) textRun {
	out := append(textRun(nil), a...)
	if len(a) > 0 && len(b) > 0 {
		out = append(out, mappedRune{value: '\n'})
	}
	return append(out, b...)
}

func render(n *treeNode, coverage *Coverage) {
	for _, child := range n.children {
		render(child, coverage)
		n.hasAmbiguous = n.hasAmbiguous || child.hasAmbiguous
	}
	base := sourceText(n.node)
	parentText := plain(base)
	positions := make([]int, len(n.children))
	present, nonempty, end := 0, 0, 0
	valid := true
	for i, child := range n.children {
		text := plain(child.rendered)
		if text == "" {
			positions[i] = -1
			continue
		}
		nonempty++
		count := strings.Count(parentText, text)
		if count != 0 {
			present++
		}
		idx := strings.Index(parentText, text)
		if count != 1 || idx < end {
			valid = false
		}
		positions[i] = idx
		if idx >= 0 {
			end = idx + len(text)
		}
	}
	if present == 0 {
		n.own = []textRun{base}
		n.rendered = append(textRun(nil), base...)
		for _, child := range n.children {
			n.rendered = join(n.rendered, child.rendered)
		}
		return
	}
	if !valid || present != nonempty {
		n.ambiguous = true
		n.hasAmbiguous = true
		n.own = []textRun{base}
		n.rendered = base
		coverage.Gaps = append(coverage.Gaps, Gap{Kind: "ambiguous_tree_text", Detail: n.node.NodeID})
		return
	}
	n.rendered = append(textRun(nil), base...)
	last := 0
	for i, child := range n.children {
		if positions[i] < 0 {
			continue
		}
		start := len([]rune(parentText[:positions[i]]))
		finish := start + len(child.rendered)
		n.own = append(n.own, n.rendered[last:start])
		for j, r := range child.rendered {
			n.rendered[start+j].positions = append(append([]sourcePos(nil), n.rendered[start+j].positions...), r.positions...)
		}
		last = finish
	}
	n.own = append(n.own, n.rendered[last:])
}

func propagateAliases(root *treeNode) {
	aliases := make(map[sourcePos][]sourcePos)
	for _, r := range root.rendered {
		for _, p := range r.positions {
			aliases[p] = r.positions
		}
	}
	var walk func(*treeNode)
	merge := func(t textRun) {
		for i := range t {
			seen := make(map[sourcePos]bool)
			for _, p := range t[i].positions {
				seen[p] = true
			}
			original := append([]sourcePos(nil), t[i].positions...)
			for _, p := range original {
				for _, alias := range aliases[p] {
					if !seen[alias] {
						t[i].positions = append(t[i].positions, alias)
						seen[alias] = true
					}
				}
			}
		}
	}
	walk = func(n *treeNode) {
		merge(n.rendered)
		for _, part := range n.own {
			merge(part)
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
}
