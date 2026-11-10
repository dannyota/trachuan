package evaluation

import "danny.vn/trachuan/internal/retrieval"

// MatchProduct returns the first rank matching an adjudicated identity.
func MatchProduct(entry MapEntry, hits []retrieval.Hit) int {
	if entry.State != "resolved" {
		return 0
	}
	for i, h := range hits {
		for _, a := range entry.Alternatives {
			if productMatch(a, h) {
				return i + 1
			}
		}
	}
	return 0
}
func productMatch(a Alternative, h retrieval.Hit) bool {
	if h.Identity.Country != a.Country {
		return false
	}
	if a.Kind == "relation" {
		if h.Identity.DocumentKey != a.SourceKey && h.Identity.DocumentKey != a.TargetKey {
			return false
		}
		for _, r := range h.Relations {
			if !r.Complete || r.TargetKey == nil || r.SourceKey != a.SourceKey || *r.TargetKey != a.TargetKey || r.Kind != a.RelationKind || r.EvidenceSHA256 != a.EvidenceSHA256 {
				continue
			}
			if a.DocumentKey != a.SourceKey && a.DocumentKey != a.TargetKey {
				continue
			}
			if a.Anchor != nil {
				endpoint := r.SourceAnchor
				if a.DocumentKey == a.TargetKey {
					endpoint = r.TargetAnchor
				}
				if endpoint == nil || *endpoint != *a.Anchor {
					continue
				}
			}
			return true
		}
		return false
	}
	if h.Identity.DocumentKey != a.DocumentKey {
		return false
	}
	if a.Kind == "document" {
		return true
	}
	if a.Kind != "provision" || a.Anchor == nil {
		return false
	}
	if h.Identity.Anchor == *a.Anchor {
		return true
	}
	for _, f := range h.Fragments {
		if f.Complete && f.Identity.Country == a.Country && f.Identity.DocumentKey == a.DocumentKey && f.Identity.Anchor == *a.Anchor {
			return true
		}
	}
	return false
}
