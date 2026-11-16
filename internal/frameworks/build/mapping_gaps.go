package build

import "danny.vn/trachuan/internal/frameworks/relate"

func releaseMappings(edges []relate.Edge, gaps []Gap) ([]relate.Edge, []Gap) {
	var retained []relate.Edge
	for _, edge := range edges {
		if edge.From.Code == edge.To.Code && edge.From.Version == edge.To.Version {
			gaps = append(gaps, Gap{Kind: "mapping_dropped", Code: edge.From.Code, Version: edge.From.Version, Count: 1})
			continue
		}
		retained = append(retained, edge)
		if !edge.Resolved {
			citation := edge.From.Citation
			gaps = append(gaps, Gap{Kind: "mapping_unresolved", Code: edge.From.Code, Version: edge.From.Version, Citation: &citation, Count: 1})
		}
	}
	type key struct{ kind, code, version, citation string }
	counts := map[key]int{}
	for _, gap := range gaps {
		citation := ""
		if gap.Citation != nil {
			citation = *gap.Citation
		}
		counts[key{gap.Kind, gap.Code, gap.Version, citation}] += gap.Count
	}
	gaps = nil
	for k, count := range counts {
		gap := Gap{Kind: k.kind, Code: k.code, Version: k.version, Count: count}
		if k.citation != "" {
			citation := k.citation
			gap.Citation = &citation
		}
		gaps = append(gaps, gap)
	}
	return retained, gaps
}
