package retrieval

import (
	"context"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

func sequenceParts(seq readSequence, index int, options ...cursor) ([]Unit, bool) {
	if parts, ok := seq.parts[index]; ok {
		return parts, true
	}
	parts, ok := unitParts(seq.units[index], options...)
	if ok && seq.parts != nil {
		seq.parts[index] = parts
	}
	return parts, ok
}

func unitParts(u Unit, options ...cursor) ([]Unit, bool) {
	var c cursor
	if len(options) > 0 {
		c = options[0]
	}
	if c.size(u) <= c.partSize() {
		return []Unit{u}, true
	}
	text := unitSplitText(u)
	if text == "" {
		return nil, false
	}
	// Recompute only when the total counter changes. Counter widths can grow,
	// so boundaries account for the counters actually serialized on each part.
	total := 1
	for {
		parts, ok := splitUnitField(u, text, total, c)
		if !ok {
			return nil, false
		}
		if len(parts) == total {
			return parts, true
		}
		total = len(parts)
	}
}

func unitSplitText(u Unit) string {
	switch u.Kind {
	case "lead", "provision":
		return u.Text
	case "outline":
		return u.Heading
	case "relation":
		if u.Relation != nil {
			return u.Relation.Evidence
		}
	}
	return ""
}

func withUnitSplitText(u Unit, text string) Unit {
	switch u.Kind {
	case "lead", "provision":
		u.Text = text
	case "outline":
		u.Heading = text
	case "relation":
		if u.Relation != nil {
			r := *u.Relation
			r.Evidence = text
			u.Relation = &r
		}
	}
	return u
}

func splitUnitField(u Unit, text string, total int, c cursor) ([]Unit, bool) {
	var parts []Unit
	for r := []rune(text); len(r) > 0; {
		base := withUnitSplitText(u, "")
		base.Part, base.Parts = len(parts)+1, total
		if c.size(base) > c.partSize() {
			return nil, false
		}
		// Find the largest encoded prefix that fits, then use the common cut rule.
		lo, hi := 0, min(len(r), c.partSize())
		for lo < hi {
			mid := (lo + hi + 1) / 2
			if c.size(withUnitSplitText(base, string(r[:mid]))) <= c.partSize() {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		if lo == 0 {
			return nil, false
		}
		end := cutAt(r, lo)
		part := withUnitSplitText(base, string(r[:end]))
		if c.size(part) > c.partSize() {
			return nil, false
		}
		parts = append(parts, part)
		r = r[end:]
	}
	return parts, true
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
	units, _ := readOutlineContext(context.Background(), nodes)
	return units
}

func readOutlineContext(ctx context.Context, nodes []Provision) ([]Unit, error) {
	articles := false
	for i, p := range nodes {
		if i%128 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if p.Kind == "dieu" {
			articles = true
			break
		}
	}
	var units []Unit
	for i, p := range nodes {
		if i%128 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		structural := p.Kind == "phan" || p.Kind == "chuong" || p.Kind == "muc" || p.Kind == "dieu"
		if (articles && structural) || (!articles && p.ParentID == nil) {
			units = append(units, Unit{Kind: "outline", Anchor: p.Identity.Anchor, Label: p.Label, Heading: p.Heading, Size: utf8.RuneCountInString(strings.TrimSpace(p.Text))})
		}
	}
	return units, ctx.Err()
}

func readTextUnit(kind string, p Provision, ancestors []Provision, text string) Unit {
	return Unit{Kind: kind, Anchor: p.Identity.Anchor, Citation: citation(ancestors), Label: p.Label, Heading: p.Heading, Text: text}
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
