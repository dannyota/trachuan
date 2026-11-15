package parse

import (
	"fmt"
	"strings"
)

func parseCIS(data []byte) ([]Node, error) {
	rows, err := rows(data, "Controls v8", true)
	if err != nil {
		return nil, err
	}
	var nodes []Node
	parents := map[string]int{}
	for _, r := range rows[min(1, len(rows)):] {
		a, b := cell(r, 0), cell(r, 1)
		if a == "" {
			continue
		}
		n := Node{Citation: a, Kind: "control", Parent: -1, Ordinal: len(nodes), PublisherTitle: cell(r, 4), Body: cell(r, 5), Status: "active"}
		if b == "" {
			parents[a] = len(nodes)
		} else {
			p, ok := parents[a]
			if !ok {
				return nodes, fmt.Errorf("missing safeguard parent")
			}
			n.Citation = b
			n.Kind = "safeguard"
			n.Parent = p
			n.Attributes.AssetType = cell(r, 2)
			n.Attributes.SecurityFunction = cell(r, 3)
			for i := 0; i < 3; i++ {
				if strings.EqualFold(cell(r, 6+i), "x") {
					n.Attributes.ImplementationGroups = append(n.Attributes.ImplementationGroups, fmt.Sprintf("IG%d", i+1))
				}
			}
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}
