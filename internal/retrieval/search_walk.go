package retrieval

type searchWalk struct {
	first               responsePlan
	primary, nonCurrent []Group
	nodes               map[int64][]string
	inside              map[Identity]bool
	outside             *int
}

func buildSearchWalk(arms Arms, recipe Recipe, firstLimit int, mode ArmMode) (searchWalk, error) {
	first, err := planWithPolicy(arms, recipe, firstLimit, mode, recipe.DocumentCap, recipe.NonCurrentTail)
	if err != nil {
		return searchWalk{}, err
	}
	w := searchWalk{first: first, nodes: arms.EffectiveNodes}
	if !arms.Validation.Accepted {
		return w, nil
	}
	primary, err := rankWithCap(arms, recipe, 100, mode, recipe.DocumentCap)
	if err != nil {
		return w, err
	}
	other, err := rankGroup(arms.NonCurrent, arms.Validation.Routed, recipe, 100, mode, recipe.DocumentCap)
	if err != nil {
		return w, err
	}
	w.primary = prefixGroups(first.primary, primary.Groups)
	w.nonCurrent = prefixGroups(first.nonCurrent, other.Groups)
	return w, nil
}

func prefixGroups(first, rest []Group) []Group {
	out := make([]Group, 0, len(first)+len(rest))
	seen := map[Identity]bool{}
	for _, gs := range [][]Group{first, rest} {
		for _, g := range gs {
			if !seen[g.Identity] {
				seen[g.Identity] = true
				out = append(out, g)
			}
		}
	}
	return out
}

func (w *searchWalk) filter(req Request, nodes map[int64][]string, docs map[string]DocumentEntry) {
	if nodes != nil {
		w.nodes = nodes
	}
	if !hasFilters(req) {
		return
	}
	w.inside = map[Identity]bool{}
	outside := 0
	for _, groups := range []*[]Group{&w.primary, &w.nonCurrent} {
		in, out := []Group{}, []Group{}
		for _, g := range *groups {
			match := matchesFilter(req, g.Identity, w.nodes[g.Best.GroupID], docs[g.Identity.DocumentKey])
			w.inside[g.Identity] = match
			if match {
				in = append(in, g)
			} else {
				outside++
				out = append(out, g)
			}
		}
		*groups = append(in, out...)
	}
	w.outside = &outside
}

func (w searchWalk) page(arms Arms, recipe Recipe, req Request, c searchCursor) (Response, error) {
	if c.TakenPrimary < 0 || c.TakenNonCurrent < 0 || c.TakenPrimary > len(w.primary) || c.TakenNonCurrent > len(w.nonCurrent) {
		return Response{}, ErrInvalidRequest
	}
	limit := req.Limit
	if limit == 0 {
		limit = FullDefaultLimit
	}
	if limit < 1 || limit > MaxLimit {
		return Response{}, ErrInvalidRequest
	}
	p, n := w.primary[c.TakenPrimary:], w.nonCurrent[c.TakenNonCurrent:]
	pc, nc := 0, 0
	first := req.Cursor == "" && c.TakenPrimary == 0 && c.TakenNonCurrent == 0
	if first {
		pc, nc = len(w.first.primary), len(w.first.nonCurrent)
	}
	if !first || hasFilters(req) {
		free := limit - pc - nc
		reserved := limit / 2
		if !arms.IncludeNonCurrent {
			reserved = min(recipe.NonCurrentTail, reserved)
		}
		extra := min(max(0, reserved-nc), len(n)-nc, free)
		nc += extra
		free -= extra
		extra = min(len(p)-pc, free)
		pc += extra
		free -= extra
		nc += min(len(n)-nc, free)
	}
	plan := responsePlan{response: w.first.response, primary: p[:pc], nonCurrent: n[:nc]}
	out, err := plan.shape(arms, recipe)
	if err != nil {
		return Response{}, err
	}
	out.OutsideFilterCount = w.outside
	for _, hits := range [][]Hit{out.Hits, out.NonCurrent} {
		for i := range hits {
			if w.inside != nil {
				v := w.inside[hits[i].Identity]
				hits[i].InFilter = &v
			}
		}
	}
	c.TakenPrimary += pc
	c.TakenNonCurrent += nc
	if c.TakenPrimary < len(w.primary) || c.TakenNonCurrent < len(w.nonCurrent) {
		out.NextCursor = encodeSearchCursor(c)
	}
	return out, nil
}
