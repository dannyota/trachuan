package retrieval

// prefixConstituents preserves the first page and removes its clauses from the
// wider ranking. A partial merge expands at its position in member rank order.
func prefixConstituents(first, rest []Group) []Group {
	out := append([]Group(nil), first...)
	seen := map[Identity]bool{}
	mark := func(g Group) {
		seen[g.Identity] = true
		if len(g.siblings) == 0 {
			return
		}
		for _, member := range g.siblings {
			seen[member.Identity] = true
		}
	}
	for _, g := range first {
		mark(g)
	}
	for _, g := range rest {
		if len(g.siblings) == 0 {
			if !seen[g.Identity] {
				out = append(out, g)
				mark(g)
			}
			continue
		}
		unlisted := make([]Group, 0, len(g.siblings))
		for _, member := range g.siblings {
			if !seen[member.Identity] {
				unlisted = append(unlisted, member)
			}
		}
		if len(unlisted) == len(g.siblings) {
			out = append(out, g)
		} else {
			out = append(out, unlisted...)
		}
		mark(g)
	}
	return out
}

func groupNodeID(g Group) int64 {
	if len(g.siblings) > 0 {
		article, _ := groupArticle(g)
		return article.ID
	}
	return g.Best.GroupID
}
