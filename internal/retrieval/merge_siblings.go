package retrieval

func groupArticle(g Group) (Provision, bool) {
	// Ancestry ends at the group, even when the best chunk belongs to a point.
	for i := len(g.Best.Ancestors) - 1; i >= 0; i-- {
		p := g.Best.Ancestors[i]
		if p.Kind == "dieu" {
			return p, true
		}
	}
	return Provision{}, false
}

func mergeSiblingGroups(groups []Group) []Group {
	type articleKey struct {
		identity Identity
		id       int64
	}
	keys := make([]articleKey, len(groups))
	clauses := map[articleKey]int{}
	veto := map[articleKey]bool{}
	for i, g := range groups {
		article, ok := groupArticle(g)
		if !ok {
			continue
		}
		key := articleKey{article.Identity, article.ID}
		keys[i] = key
		if g.Best.GroupID == article.ID {
			veto[key] = true
		}
		for _, p := range g.Best.Ancestors {
			if p.ID == g.Best.GroupID && p.Kind == "khoan" {
				clauses[key]++
			}
		}
	}
	out := make([]Group, 0, len(groups))
	indices := map[articleKey]int{}
	for i, g := range groups {
		key := keys[i]
		isClause := false
		for _, p := range g.Best.Ancestors {
			if p.ID == g.Best.GroupID && p.Kind == "khoan" {
				isClause = true
			}
		}
		if !isClause || clauses[key] < 2 || veto[key] {
			out = append(out, g)
			continue
		}
		if index, ok := indices[key]; ok {
			target := &out[index]
			target.siblings = append(target.siblings, g)
			target.Members = append(target.Members, g.Members...)
			target.Score = max(target.Score, g.Score)
			target.BestCosine = max(target.BestCosine, g.BestCosine)
		} else {
			indices[key] = len(out)
			merged := g
			merged.Identity = key.identity
			merged.Members = append([]Candidate(nil), g.Members...)
			merged.siblings = []Group{g}
			out = append(out, merged)
		}
	}
	return out
}
