package source

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// ListingItem holds the listing fields used for local selection.
type ListingItem struct {
	ID        string
	Title     string
	AgencyIDs []string
}

// SelectionRule identifies a rule supporting a selected item.
type SelectionRule struct{ Kind, Value string }

// SelectedItem holds the rules supporting a listing item.
type SelectedItem struct {
	ID    string
	Rules []SelectionRule
}

// Select applies frozen local rules and returns items and supports in sorted order.
func Select(items []ListingItem, rules VBPLSources) ([]SelectedItem, error) {
	if rules.SelectionVersion != 1 {
		return nil, fmt.Errorf("unsupported selection version %d", rules.SelectionVersion)
	}
	var out []SelectedItem
	for _, item := range items {
		if strings.HasPrefix(item.ID, "vbpqta_") {
			continue
		}
		matches := make(map[SelectionRule]bool)
		for _, agency := range rules.SweepAgencies {
			for _, id := range item.AgencyIDs {
				if agency == id {
					matches[SelectionRule{"agency", agency}] = true
				}
			}
		}
		title := normalizeTitle(item.Title)
		for _, term := range rules.Keywords {
			normalized := normalizeTitle(term)
			if normalized != "" && strings.Contains(title, normalized) {
				matches[SelectionRule{"term", term}] = true
			}
		}
		if len(matches) == 0 {
			continue
		}
		selected := SelectedItem{ID: item.ID}
		for match := range matches {
			selected.Rules = append(selected.Rules, match)
		}
		sort.Slice(selected.Rules, func(i, j int) bool {
			a, b := selected.Rules[i], selected.Rules[j]
			if a.Kind != b.Kind {
				return a.Kind < b.Kind
			}
			return a.Value < b.Value
		})
		out = append(out, selected)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func normalizeTitle(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(norm.NFC.String(s))), " ")
}
