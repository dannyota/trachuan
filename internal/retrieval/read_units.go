package retrieval

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

func splitParts(r []rune, partSize int) [][]rune {
	var parts [][]rune
	for len(r) > 0 {
		end := cutAt(r, partSize)
		parts = append(parts, r[:end])
		r = r[end:]
	}
	return parts
}

func unitParts(u Unit) []Unit {
	if readSize(u) <= PartSize {
		return []Unit{u}
	}
	// Reserve the largest possible part counters before choosing boundaries.
	// JSON escaping counts too, so a source quote can cost two code points.
	u.Part, u.Parts = readSize(u), readSize(u)
	field := 0
	for i := 1; i < 6; i++ {
		if len(unitField(u, i)) > len(unitField(u, field)) {
			field = i
		}
	}
	base := withUnitField(u, field, "")
	var parts []Unit
	if readSize(base) < PartSize && u.Relation == nil {
		parts = splitUnitField(base, field, unitField(u, field))
	} else {
		// Oversized labels or headings are source fields too. When the metadata
		// cannot ride on every part, deliver each field once in field order.
		base = u
		for i := 0; i < 6; i++ {
			base = withUnitField(base, i, "")
		}
		for i := 0; i < 6; i++ {
			parts = append(parts, splitUnitField(base, i, unitField(u, i))...)
		}
	}
	if len(parts) == 0 {
		parts = []Unit{u}
	}
	for i := range parts {
		parts[i].Part, parts[i].Parts = i+1, len(parts)
	}
	return parts
}

func unitField(u Unit, field int) string {
	switch field {
	case 0:
		return u.Text
	case 1:
		return u.Heading
	case 2:
		return u.Label
	case 3:
		return u.Citation
	case 4:
		if u.Relation != nil {
			return u.Relation.RawLabel
		}
	case 5:
		if u.Relation != nil {
			return u.Relation.Evidence
		}
	}
	return ""
}

func withUnitField(u Unit, field int, text string) Unit {
	switch field {
	case 0:
		u.Text = text
	case 1:
		u.Heading = text
	case 2:
		u.Label = text
	case 3:
		u.Citation = text
	case 4, 5:
		if u.Relation != nil {
			r := *u.Relation
			if field == 4 {
				r.RawLabel = text
			} else {
				r.Evidence = text
			}
			u.Relation = &r
		}
	}
	return u
}

func splitUnitField(base Unit, field int, text string) []Unit {
	var parts []Unit
	for r := []rune(text); len(r) > 0; {
		// Find the largest encoded prefix that fits, then use the common cut rule.
		lo, hi := 1, min(len(r), PartSize)
		for lo < hi {
			mid := (lo + hi + 1) / 2
			if readSize(withUnitField(base, field, string(r[:mid]))) <= PartSize {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		span := splitParts(r[:min(len(r), lo+1)], lo)[0]
		parts = append(parts, withUnitField(base, field, string(span)))
		r = r[len(span):]
	}
	return parts
}

func readAncestry(nodes []Provision, p Provision) []Provision {
	byID := make(map[int64]Provision, len(nodes))
	for _, node := range nodes {
		byID[node.ID] = node
	}
	var chain []Provision
	seen := map[int64]bool{}
	for !seen[p.ID] {
		seen[p.ID] = true
		chain = append(chain, p)
		if p.ParentID == nil {
			break
		}
		parent, ok := byID[*p.ParentID]
		if !ok {
			break
		}
		p = parent
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

func readDescendants(nodes []Provision, root int64) []Provision {
	included := map[int64]bool{root: true}
	var out []Provision
	for _, p := range nodes {
		if p.ID == root || (p.ParentID != nil && included[*p.ParentID]) {
			included[p.ID] = true
			out = append(out, p)
		}
	}
	return out
}

func readOutline(nodes []Provision) []Unit {
	articles := false
	for _, p := range nodes {
		if p.Kind == "dieu" {
			articles = true
			break
		}
	}
	var units []Unit
	for _, p := range nodes {
		structural := p.Kind == "phan" || p.Kind == "chuong" || p.Kind == "muc" || p.Kind == "dieu"
		if (articles && structural) || (!articles && p.ParentID == nil) {
			units = append(units, Unit{Kind: "outline", Handle: Handle(p.Identity), Label: p.Label, Heading: p.Heading, Size: utf8.RuneCountInString(strings.TrimSpace(p.Text))})
		}
	}
	return units
}

func readTextUnit(kind string, p Provision, ancestors []Provision, text string) Unit {
	return Unit{Kind: kind, Handle: Handle(p.Identity), Citation: citation(ancestors), Label: p.Label, Heading: p.Heading, Text: text}
}

func readOwnText(nodes []Provision, p Provision) (string, []Gap) {
	raw := strings.TrimSpace(p.Text)
	if renderTree(readDescendants(nodes, p.ID), p.ID).ambiguous {
		return raw, []Gap{readAmbiguity(p)}
	}
	var own strings.Builder
	last := 0
	for _, child := range nodes {
		if child.ParentID == nil || *child.ParentID != p.ID {
			continue
		}
		text := string(renderTree(readDescendants(nodes, child.ID), child.ID).text)
		if text == "" {
			continue
		}
		at := strings.Index(raw, text)
		if at < 0 {
			continue
		}
		own.WriteString(raw[last:at])
		last = at + len(text)
	}
	own.WriteString(raw[last:])
	return strings.TrimSpace(own.String()), nil
}

func readLeads(nodes []Provision, target Provision) ([]Unit, []Gap) {
	chain := readAncestry(nodes, target)
	var units []Unit
	var gaps []Gap
	for i, p := range chain[:len(chain)-1] {
		if p.Kind != "dieu" && p.Kind != "khoan" && p.Kind != "diem" {
			continue
		}
		tree := renderTree(readDescendants(nodes, p.ID), p.ID)
		if tree.ambiguous {
			gaps = append(gaps, readAmbiguity(p))
			continue
		}
		lead := strings.TrimSpace(p.Text)
		// renderTree owns containment decisions. The earliest child fragment marks
		// the end of the parent's own lead when children occur inside its text.
		end := len([]rune(lead))
		for _, f := range tree.fragments {
			if f.Identity != p.Identity && f.TextStart < end {
				end = f.TextStart
			}
		}
		lead = strings.TrimSpace(string([]rune(lead)[:end]))
		if lead != "" && !readLabelOnly(lead, p.Label) {
			units = append(units, readTextUnit("lead", p, chain[:i+1], lead))
		}
	}
	return units, gaps
}

func readLabelOnly(text, label string) bool {
	normalize := func(s string) string {
		return strings.TrimRight(strings.TrimSpace(strings.ToLower(norm.NFC.String(s))), ".:)")
	}
	return label != "" && normalize(text) == normalize(label)
}

func readAmbiguity(p Provision) Gap {
	return Gap{Kind: "ambiguous_tree_text", DocumentKey: p.Identity.DocumentKey, Anchor: p.Identity.Anchor, Message: "Source tree text overlaps ambiguously."}
}
