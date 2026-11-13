package retrieval

func setFragmentCitations(fragments []Fragment, candidates []Candidate) {
	citations := map[Identity]string{}
	for _, c := range candidates {
		paths := map[int64][]Provision{}
		for i, p := range c.Ancestors {
			paths[p.ID] = c.Ancestors[:i+1]
			citations[p.Identity] = citation(paths[p.ID])
		}
		nodes := map[int64]Provision{}
		for _, p := range c.Descendants {
			nodes[p.ID] = p
		}
		var path func(int64) []Provision
		path = func(id int64) []Provision {
			if p, ok := paths[id]; ok {
				return p
			}
			p, ok := nodes[id]
			if !ok {
				return nil
			}
			var ancestors []Provision
			if p.ParentID != nil {
				ancestors = path(*p.ParentID)
			}
			ancestors = append(append([]Provision(nil), ancestors...), p)
			paths[id] = ancestors
			return ancestors
		}
		for _, p := range c.Descendants {
			citations[p.Identity] = citation(path(p.ID))
		}
	}
	for i := range fragments {
		fragments[i].Citation = citations[fragments[i].Identity]
	}
}
