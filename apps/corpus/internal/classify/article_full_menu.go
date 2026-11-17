package classify

import "sort"

func fullArticleMenu(nodes []Node) []Node {
	byCode := map[string]Node{}
	hasChild := map[string]bool{}
	for _, n := range nodes {
		byCode[n.Code] = n
		if n.Parent != "" {
			hasChild[n.Parent] = true
		}
	}
	var menu []Node
	for _, n := range nodes {
		parent := byCode[n.Parent]
		if n.Level == 2 && n.Active && parent.Active && parent.Level == 1 && (n.HasProvisions || hasChild[n.Code]) {
			n.Name = parent.Name + " > " + n.Name
			menu = append(menu, n)
		}
	}
	sort.Slice(menu, func(i, j int) bool {
		a, b := menu[i], menu[j]
		pa, pb := byCode[a.Parent], byCode[b.Parent]
		if pa.Ordinal != pb.Ordinal {
			return pa.Ordinal < pb.Ordinal
		}
		if a.Ordinal != b.Ordinal {
			return a.Ordinal < b.Ordinal
		}
		return a.Code < b.Code
	})
	return menu
}
