package fetch

import (
	"sort"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/internal/retrieval"
)

type handOverIdentity struct{ number, issuer, date string }
type handOverPair struct{ key, state string }
type handOverDecision struct {
	kind string
	keys []string
}

func normalizedIdentity(number, issuer, date string) handOverIdentity {
	return handOverIdentity{retrieval.NormalizeNumber(number), source.NormalizeIssuer(issuer), normalizeDate(date)}
}
func identityConflict(a, b handOverIdentity) bool {
	return a.number != "" && b.number != "" && a.number != b.number || a.issuer != "" && b.issuer != "" && a.issuer != b.issuer || a.date != "" && b.date != "" && a.date != b.date
}
func compareHandOver(a, b handOverIdentity) string {
	if a.number != "" && b.number != "" && a.number != b.number {
		return "different_number"
	}
	if a.number == "" || b.number == "" {
		return "ambiguous"
	}
	if a.date != "" && b.date != "" && a.date != b.date {
		return "different_date"
	}
	if a.date == "" || b.date == "" || a.issuer == "" || b.issuer == "" || a.issuer != b.issuer {
		return "ambiguous"
	}
	return "match"
}
func decideHandOver(pairs []handOverPair) handOverDecision {
	var out handOverDecision
	for _, pair := range pairs {
		if pair.state == "different_date" || pair.state == "different_number" {
			continue
		}
		out.keys = append(out.keys, pair.key)
		if pair.state != "match" {
			out.kind = "source_identity_ambiguous"
		}
	}
	sort.Strings(out.keys)
	if len(out.keys) == 0 {
		return out
	}
	if len(out.keys) > 1 {
		out.kind = "source_identity_ambiguous"
	}
	if out.kind == "" {
		out.kind = "source_replaced"
	}
	return out
}
