package taxonomy

// ArticleResolver resolves package articles using indexes built once.
type ArticleResolver struct {
	nodes     map[string]MappedNode
	codes     map[string]MappedNode
	tree      map[string]TreeNode
	flattened map[string]string
}

// NewArticleResolver builds indexes for article resolution.
func NewArticleResolver(pkg Package, m Mapping) *ArticleResolver {
	r := &ArticleResolver{nodes: make(map[string]MappedNode, len(m.Nodes)), codes: make(map[string]MappedNode, len(m.Nodes)), tree: make(map[string]TreeNode, len(pkg.Tree)), flattened: make(map[string]string, len(m.Flattened))}
	for _, n := range m.Nodes {
		if n.Status == "active" || r.nodes[n.GUID].GUID == "" {
			r.nodes[n.GUID] = n
		}
		r.codes[n.Code] = n
	}
	for _, n := range pkg.Tree {
		r.tree[n.GUID] = n
	}
	for _, f := range m.Flattened {
		r.flattened[f.GUID] = f.Into
	}
	return r
}

// Code returns the deepest active code for a placed article.
func (r *ArticleResolver) Code(a Article) (string, bool) {
	if a.TreeGUID == "" {
		return "", false
	}
	d, ok := r.nodes[a.DeMucGUID]
	if !ok || d.Status != "active" {
		return "", false
	}
	id := a.TreeGUID
	seen := map[string]bool{}
	for id != "" && !seen[id] {
		seen[id] = true
		n, exists := r.tree[id]
		if !exists || n.DeMucGUID != a.DeMucGUID {
			return "", false
		}
		if code, found := r.flattened[id]; found {
			target, ok := r.codes[code]
			if !ok || target.Status != "active" {
				return "", false
			}
			return code, true
		}
		if mapped, found := r.nodes[id]; found {
			if mapped.Status != "active" {
				return "", false
			}
			return mapped.Code, true
		}
		id = n.ParentGUID
	}
	return d.Code, true
}
