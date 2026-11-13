package evaluation

import (
	"strings"
	"unicode"

	"danny.vn/trachuan/internal/retrieval"
)

func numberMatches(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	if strings.EqualFold(a, b) {
		return true
	}
	normalize := func(s string) string {
		var out strings.Builder
		for _, r := range strings.ToUpper(s) {
			if r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
				out.WriteRune(r)
			}
		}
		return out.String()
	}
	a = normalize(a)
	b = normalize(b)
	return a != "" && b != "" && (strings.Contains(a, b) || strings.Contains(b, a))
}
func citationMatches(e Expectation, citation string) bool {
	fields := strings.FieldsFunc(citation, func(r rune) bool { return r == ',' || unicode.IsSpace(r) })
	for _, v := range []struct{ label, value string }{{"điều", e.Article}, {"khoản", e.Clause}, {"điểm", e.Point}} {
		if v.value == "" {
			continue
		}
		found := false
		for i := 0; i+1 < len(fields); i++ {
			if strings.EqualFold(fields[i], v.label) && strings.EqualFold(strings.Trim(fields[i+1], "()."), strings.Trim(v.value, "().")) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// MatchLegacy returns the first matching rank and whether it used a relation.
func MatchLegacy(e Expectation, hits []retrieval.Hit) (int, bool) {
	numbers := append([]string{e.DocNumber}, e.AltDocNumbers...)
	for i, h := range hits {
		for _, n := range numbers {
			if numberMatches(n, h.Number) {
				if citationMatches(e, h.Citation) {
					return i + 1, false
				}
				for _, fragment := range h.Fragments {
					if fragment.Complete && citationMatches(e, fragment.Citation) {
						return i + 1, false
					}
				}
			}
		}
		if !e.RelationOK || e.Article != "" || e.Clause != "" || e.Point != "" {
			continue
		}
		for _, rel := range h.Relations {
			if !rel.SourceResolved || !rel.TargetResolved {
				continue
			}
			other := ""
			if rel.SourceKey == h.Identity.DocumentKey {
				other = rel.TargetNumber
			} else if rel.TargetKey != nil && *rel.TargetKey == h.Identity.DocumentKey {
				other = rel.SourceNumber
			}
			for _, n := range numbers {
				if numberMatches(n, other) {
					return i + 1, true
				}
			}
		}
	}
	return 0, false
}
