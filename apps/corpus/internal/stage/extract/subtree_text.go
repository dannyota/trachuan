package extract

import (
	"sort"
	"strings"
)

type subtreeText struct {
	rendered, full string
	ambiguous      bool
}

// renderTreeText follows chunk's rendering, including the own spans and child
// subtrees chunk processes separately when containment is ambiguous.
func renderTreeText(rows []provision) map[string]string {
	children := make(map[string][]provision)
	for _, row := range rows {
		if row.ParentNodeID != nil {
			children[*row.ParentNodeID] = append(children[*row.ParentNodeID], row)
		}
	}
	for _, siblings := range children {
		sort.Slice(siblings, func(i, j int) bool {
			if siblings[i].Ordinal != siblings[j].Ordinal {
				return siblings[i].Ordinal < siblings[j].Ordinal
			}
			return siblings[i].NodeID < siblings[j].NodeID
		})
	}
	out := make(map[string]string, len(rows))
	var render func(provision) subtreeText
	render = func(row provision) subtreeText {
		base := strings.TrimSpace(row.Text)
		var childTexts []subtreeText
		for _, child := range children[row.NodeID] {
			childTexts = append(childTexts, render(child))
		}
		text := renderSubtreeText(base, childTexts)
		out[row.NodeID] = text.full
		return text
	}
	for _, row := range rows {
		if row.ParentNodeID == nil {
			render(row)
		}
	}
	return out
}

func renderSubtreeText(base string, children []subtreeText) subtreeText {
	result := subtreeText{rendered: base}
	present, nonempty, end := 0, 0, 0
	valid := true
	positions := make([]int, len(children))
	var rendered []string
	rendered = append(rendered, base)
	for i, child := range children {
		result.ambiguous = result.ambiguous || child.ambiguous
		text := child.rendered
		positions[i] = -1
		if text == "" {
			continue
		}
		rendered = append(rendered, text)
		nonempty++
		count := strings.Count(base, text)
		if count != 0 {
			present++
		}
		start := strings.Index(base, text)
		if count != 1 || start < end {
			valid = false
		}
		positions[i] = start
		if start >= 0 {
			end = start + len(text)
		}
	}
	var own []string
	switch {
	case present == 0:
		own = []string{base}
		result.rendered = joinSubtreeText(rendered)
	case !valid || present != nonempty:
		result.ambiguous = true
		own = []string{base}
	default:
		last := 0
		for i, child := range children {
			if positions[i] < 0 {
				continue
			}
			own = append(own, base[last:positions[i]])
			last = positions[i] + len(child.rendered)
		}
		own = append(own, base[last:])
	}
	result.full = result.rendered
	if result.ambiguous {
		for _, child := range children {
			own = append(own, child.full)
		}
		result.full = joinSubtreeText(own)
	}
	return result
}

func joinSubtreeText(parts []string) string {
	nonempty := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			nonempty = append(nonempty, part)
		}
	}
	return strings.Join(nonempty, "\n")
}
