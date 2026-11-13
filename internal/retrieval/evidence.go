package retrieval

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"unicode"
)

type sourceSpan struct {
	NodeID       string `json:"node_id"`
	Start        int    `json:"start"`
	End          int    `json:"end"`
	ContentStart int    `json:"content_start"`
	ContentEnd   int    `json:"content_end"`
	Role         string `json:"role"`
}

type rendered struct {
	text      []rune
	fragments []Fragment
	ambiguous bool
}

func citation(ancestors []Provision) string {
	labels := []string{}
	for _, p := range ancestors {
		if (p.Kind == "dieu" || p.Kind == "khoan" || p.Kind == "diem") && strings.TrimSpace(p.Label) != "" {
			labels = append(labels, strings.TrimSpace(p.Label))
		}
	}
	if len(labels) == 0 && len(ancestors) > 0 {
		return strings.TrimSpace(ancestors[len(ancestors)-1].Label)
	}
	return strings.Join(labels, ", ")
}

func evidence(g Group, capRunes int) (string, []Fragment, bool, []Gap, Identity, int, error) {
	if len(g.siblings) > 0 {
		return siblingEvidence(g, capRunes)
	}
	c := g.Best
	foundRoot := false
	for _, p := range c.Descendants {
		if p.ID == c.GroupID {
			foundRoot = true
			break
		}
	}
	if !foundRoot {
		return "", nil, false, nil, Identity{}, 0, ErrCacheInsufficient
	}
	if capRunes < 1 {
		return "", nil, false, nil, Identity{}, 0, errors.New("invalid evidence cap")
	}
	byNode := map[string]Provision{}
	for _, p := range c.Ancestors {
		byNode[p.NodeID] = p
	}
	for _, p := range c.Descendants {
		byNode[p.NodeID] = p
	}
	var spans []sourceSpan
	if len(c.Spans) > 0 && json.Unmarshal(c.Spans, &spans) != nil {
		return "", nil, false, nil, Identity{}, 0, errors.New("invalid evidence spans")
	}
	leads := rendered{}
	for _, ancestor := range c.Ancestors {
		if ancestor.ID == c.GroupID {
			continue
		}
		part, err := renderSpans(c.Content, spans, byNode, "lead", ancestor.NodeID)
		if err != nil {
			return "", nil, false, nil, Identity{}, 0, err
		}
		appendRendered(&leads, part)
	}
	bestMember := c.Identity
	var bestSpan *sourceSpan
	for i := range spans {
		span := &spans[i]
		if span.Role == "body" && (bestSpan == nil || span.ContentStart < bestSpan.ContentStart) {
			bestSpan = span
		}
	}
	if bestSpan != nil {
		if p, ok := byNode[bestSpan.NodeID]; ok {
			bestMember = p.Identity
		}
	}
	tree := renderTree(c.Descendants, c.GroupID)
	full := rendered{}
	appendRendered(&full, leads)
	appendRendered(&full, tree)
	expected := append([]Fragment(nil), full.fragments...)
	complete := len(full.text) <= capRunes
	if !complete {
		full = rendered{}
		appendRendered(&full, leads)
		body, err := renderSpans(c.Content, spans, byNode, "body", "")
		if err != nil {
			return "", nil, false, nil, Identity{}, 0, err
		}
		appendRendered(&full, body)
	}
	var gaps []Gap
	if tree.ambiguous {
		gaps = append(gaps, Gap{Kind: "ambiguous_tree_text", DocumentKey: g.Identity.DocumentKey, Anchor: g.Identity.Anchor, Message: "Source tree text overlaps ambiguously."})
	}
	if len(full.text) > capRunes {
		full.text = full.text[:capRunes]
	}
	fragments := full.fragments[:0]
	for _, fragment := range full.fragments {
		if fragment.TextStart < len(full.text) {
			fragments = append(fragments, fragment)
		}
	}
	full.fragments = fragments
	for i := range full.fragments {
		f := &full.fragments[i]
		if f.TextEnd > len(full.text) {
			f.TextEnd = len(full.text)
			f.End = f.Start + max(0, f.TextEnd-f.TextStart)
			f.Complete = false
		}
		if f.TextStart >= f.TextEnd {
			f.Complete = false
		}
	}
	for i := range full.fragments {
		f := &full.fragments[i]
		for _, covered := range expected {
			if covered.Identity == f.Identity && (covered.Start < f.Start || covered.End > f.End) {
				f.Complete = false
			}
		}
	}
	SetFragmentCitations(full.fragments, []Candidate{c})
	bestStart := 0
	located := false
	if bestSpan != nil {
		for _, f := range full.fragments {
			if f.Role == "body" && f.Identity == bestMember && f.Start <= bestSpan.Start && bestSpan.Start < f.End {
				bestStart = f.TextStart + bestSpan.Start - f.Start
				located = true
				break
			}
		}
	}
	if !located {
		bestMember = g.Identity
		if len(leads.text) > 0 {
			bestStart = min(len(leads.text)+1, len(full.text))
		}
	}
	return string(full.text), full.fragments, complete, gaps, bestMember, bestStart, nil
}

func appendRendered(out *rendered, part rendered) {
	if len(part.text) == 0 {
		return
	}
	if len(out.text) > 0 {
		out.text = append(out.text, '\n')
	}
	shift := len(out.text)
	out.text = append(out.text, part.text...)
	for _, f := range part.fragments {
		f.TextStart += shift
		f.TextEnd += shift
		out.fragments = append(out.fragments, f)
	}
	out.ambiguous = out.ambiguous || part.ambiguous
}

func renderTree(nodes []Provision, root int64) rendered {
	byID := map[int64]Provision{}
	children := map[int64][]Provision{}
	for _, p := range nodes {
		byID[p.ID] = p
		if p.ParentID != nil {
			children[*p.ParentID] = append(children[*p.ParentID], p)
		}
	}
	for id := range children {
		sort.Slice(children[id], func(i, j int) bool {
			a, b := children[id][i], children[id][j]
			if a.Ordinal != b.Ordinal {
				return a.Ordinal < b.Ordinal
			}
			return a.NodeID < b.NodeID
		})
	}
	var visit func(int64) rendered
	visit = func(id int64) rendered {
		p, ok := byID[id]
		if !ok {
			return rendered{}
		}
		raw := []rune(p.Text)
		start, end := 0, len(raw)
		for start < end && unicode.IsSpace(raw[start]) {
			start++
		}
		for end > start && unicode.IsSpace(raw[end-1]) {
			end--
		}
		own := rendered{text: append([]rune(nil), raw[start:end]...)}
		if end > start {
			own.fragments = []Fragment{{Identity: p.Identity, Start: start, End: end, TextStart: 0, TextEnd: end - start, Role: "body", Complete: true}}
		}
		parts := []rendered{}
		for _, child := range children[id] {
			part := visit(child.ID)
			if len(part.text) > 0 {
				parts = append(parts, part)
			}
		}
		found, last := 0, 0
		positions := make([]int, len(parts))
		parent := string(own.text)
		for i, part := range parts {
			needle := string(part.text)
			if strings.Count(parent, needle) == 1 {
				pos := strings.Index(parent, needle)
				runePos := len([]rune(parent[:pos]))
				positions[i] = runePos
				if runePos >= last {
					found++
					last = runePos + len(part.text)
				}
			}
		}
		if found == len(parts) {
			for i, part := range parts {
				for _, f := range part.fragments {
					f.TextStart += positions[i]
					f.TextEnd += positions[i]
					own.fragments = append(own.fragments, f)
				}
				own.ambiguous = own.ambiguous || part.ambiguous
			}
			return own
		}
		if found == 0 {
			any := false
			for _, part := range parts {
				if strings.Contains(parent, string(part.text)) {
					any = true
					break
				}
			}
			if !any {
				for _, part := range parts {
					appendRendered(&own, part)
				}
				return own
			}
		}
		own.ambiguous = true
		return own
	}
	return visit(root)
}
