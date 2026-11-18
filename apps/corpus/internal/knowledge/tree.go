package knowledge

import (
	"slices"
	"strings"
)

type tree struct {
	byAnchor map[string]*Provision
	byID     map[int64]*Provision
	suffixed map[string]bool
}

func newTree(provisions []Provision) *tree {
	t := &tree{byAnchor: make(map[string]*Provision, len(provisions)), byID: make(map[int64]*Provision, len(provisions)), suffixed: map[string]bool{}}
	for i := range provisions {
		p := &provisions[i]
		t.byAnchor[p.Anchor] = p
		t.byID[p.ID] = p
		prefix := ""
		for _, component := range strings.Split(p.Anchor, "/") {
			if base, ok := cutSuffix(component); ok {
				t.suffixed[prefix+base] = true
			}
			prefix += component + "/"
		}
	}
	return t
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

func (t *tree) lineage(n *Provision) []*Provision {
	chain := []*Provision{n}
	for current := n; current.ParentID != nil && len(chain) <= len(t.byID); {
		parent := t.byID[*current.ParentID]
		if parent == nil {
			break
		}
		chain = append(chain, parent)
		current = parent
	}
	slices.Reverse(chain)
	return chain
}

func (t *tree) ancestors(n *Provision) []Ancestor {
	chain := t.lineage(n)
	out := make([]Ancestor, 0, len(chain)-1)
	for _, p := range chain[:len(chain)-1] {
		out = append(out, Ancestor{Anchor: p.Anchor, HeadingSHA256: Hash(p.Heading)})
	}
	return out
}

func (t *tree) ambiguous(n *Provision) bool {
	for _, p := range t.lineage(n) {
		prefix := ""
		for _, component := range strings.Split(p.Anchor, "/") {
			if _, suffixed := cutSuffix(component); suffixed || t.suffixed[prefix+component] {
				return true
			}
			prefix += component + "/"
		}
	}
	return false
}
