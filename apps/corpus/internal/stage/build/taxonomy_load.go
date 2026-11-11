package build

import (
	"context"
	"fmt"
	"sort"

	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
)

type taxonomyNode struct {
	id, parent, family                                        int64
	code, parentCode, name, description, origin, status, path string
	level, ordinal, order                                     int
}

func taxonomyNodes(input TaxonomyInput) ([]taxonomyNode, error) {
	all := map[string]*taxonomyNode{}
	ordered := taxonomy.Mapping{}
	for _, n := range input.mapping.Nodes {
		all[n.Code] = &taxonomyNode{code: n.Code, parentCode: n.ParentCode, name: n.Name, origin: "phapdien", status: n.Status, level: n.Level, order: n.Ordinal}
		ordered.Nodes = append(ordered.Nodes, n)
	}
	for i, n := range input.custom {
		all[n.Code] = &taxonomyNode{code: n.Code, parentCode: n.ParentCode, name: n.Name, description: n.Description, origin: "custom", status: "active", order: i}
		ordered.Nodes = append(ordered.Nodes, taxonomy.MappedNode{Code: n.Code})
	}
	var err error
	ordered, err = taxonomy.ReadMapping(ordered.Encode())
	if err != nil {
		return nil, fmt.Errorf("order taxonomy nodes: %w", err)
	}
	state := map[string]int{}
	var resolve func(string) (bool, error)
	resolve = func(code string) (bool, error) {
		n := all[code]
		if n == nil {
			return false, fmt.Errorf("missing taxonomy node %s", code)
		}
		if state[code] == 1 {
			return false, fmt.Errorf("cyclic taxonomy node %s", code)
		}
		if state[code] == 2 {
			return n.level > 0, nil
		}
		state[code] = 1
		if n.parentCode != "" {
			ok, e := resolve(n.parentCode)
			if e != nil {
				return false, e
			}
			if !ok {
				n.level = 0
			} else {
				n.level = all[n.parentCode].level + 1
				if n.level > 4 {
					n.level = 0
				}
			}
		} else {
			n.level = 1
		}
		state[code] = 2
		return n.level > 0, nil
	}
	var nodes []taxonomyNode
	for _, n := range ordered.Nodes {
		ok, e := resolve(n.Code)
		if e != nil {
			return nil, e
		}
		if ok {
			nodes = append(nodes, *all[n.Code])
		}
	}
	for i := range nodes {
		nodes[i].id = int64(i + 1)
		all[nodes[i].code].id = nodes[i].id
	}
	siblings := map[string][]*taxonomyNode{}
	for i := range nodes {
		n := &nodes[i]
		siblings[n.parentCode] = append(siblings[n.parentCode], n)
	}
	rank := func(n *taxonomyNode) int {
		if n.status == "retired" {
			return 2
		}
		if n.origin == "custom" {
			return 1
		}
		return 0
	}
	for _, group := range siblings {
		sort.SliceStable(group, func(i, j int) bool {
			a, b := group[i], group[j]
			if rank(a) != rank(b) {
				return rank(a) < rank(b)
			}
			if rank(a) == 2 {
				return false
			}
			return a.order < b.order
		})
		for i, n := range group {
			n.ordinal = i + 1
		}
	}
	index := map[string]*taxonomyNode{}
	for i := range nodes {
		index[nodes[i].code] = &nodes[i]
	}
	var fill func(*taxonomyNode)
	fill = func(n *taxonomyNode) {
		if n.path != "" {
			return
		}
		n.path = fmt.Sprintf("n%d", n.id)
		if n.parentCode != "" {
			p := index[n.parentCode]
			fill(p)
			n.parent = p.id
			n.path = p.path + "." + n.path
			n.family = p.family
		}
		if n.level == 2 {
			n.family = n.id
		}
	}
	for i := range nodes {
		fill(&nodes[i])
	}
	return nodes, nil
}

func (l *loader) loadTaxonomyNodes(ctx context.Context, input TaxonomyInput) (map[string]taxonomyNode, error) {
	nodes, err := taxonomyNodes(input)
	if err != nil {
		return nil, err
	}
	byCode := map[string]taxonomyNode{}
	for _, n := range nodes {
		byCode[n.code] = n
	}
	sort.SliceStable(nodes, func(i, j int) bool { return nodes[i].level < nodes[j].level })
	for _, n := range nodes {
		var parent any
		if n.parent != 0 {
			parent = n.parent
		}
		_, err = l.tx.Exec(ctx, `INSERT INTO taxonomy_node(id,country,parent_id,level,code,name,description,origin,status,path,ordinal,provision_count,document_count) VALUES($1,'vn',$2,$3,$4,$5,$6,$7,$8,$9::ltree,$10,0,0)`, n.id, parent, n.level, n.code, n.name, n.description, n.origin, n.status, n.path, n.ordinal)
		if err != nil {
			return nil, fmt.Errorf("insert taxonomy node: %w", err)
		}
	}
	l.counts.Taxonomy.Nodes = int64(len(nodes))
	return byCode, nil
}
