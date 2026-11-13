package retrieval

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"unicode"
)

type siblingPart struct {
	group Group
	tree  rendered
	order int
}

func siblingEvidence(g Group, capRunes int) (string, []Fragment, bool, []Gap, Identity, int, error) {
	if capRunes < 1 {
		return "", nil, false, nil, Identity{}, 0, errors.New("invalid evidence cap")
	}
	if g.article == nil {
		return "", nil, false, nil, Identity{}, 0, ErrCacheInsufficient
	}
	winner := g.siblings[0].Best
	byNode := map[string]Provision{}
	for _, p := range winner.Ancestors {
		byNode[p.NodeID] = p
	}
	for _, p := range winner.Descendants {
		byNode[p.NodeID] = p
	}
	var spans []sourceSpan
	if len(winner.Spans) > 0 && json.Unmarshal(winner.Spans, &spans) != nil {
		return "", nil, false, nil, Identity{}, 0, errors.New("invalid evidence spans")
	}
	leads := rendered{}
	for _, p := range winner.Ancestors {
		if p.ID == winner.GroupID {
			continue
		}
		part, err := renderSpans(winner.Content, spans, byNode, "lead", p.NodeID)
		if err != nil {
			return "", nil, false, nil, Identity{}, 0, err
		}
		appendRendered(&leads, part)
	}
	memberIdentities := make(map[Identity]bool, len(g.siblings))
	for _, member := range g.siblings {
		memberIdentities[member.Identity] = true
	}
	parts := make([]siblingPart, 0, len(g.siblings))
	for _, member := range g.siblings {
		found := false
		for _, p := range member.Best.Descendants {
			if p.ID == member.Best.GroupID {
				found = true
			}
		}
		if !found {
			return "", nil, false, nil, Identity{}, 0, ErrCacheInsufficient
		}
		order := 0
		for i, p := range g.article.Descendants {
			if p.ID == member.Best.GroupID {
				order = i
				break
			}
		}
		tree := renderTree(member.Best.Descendants, member.Best.GroupID)
		for _, node := range member.Best.Descendants {
			if node.Kind == "khoan" && !memberIdentities[node.Identity] {
				for i := range tree.fragments {
					if tree.fragments[i].Identity == node.Identity {
						tree.fragments[i].Complete = false
					}
				}
			}
		}
		parts = append(parts, siblingPart{member, tree, order})
	}
	source := append([]siblingPart(nil), parts...)
	sort.SliceStable(source, func(i, j int) bool { return source[i].order < source[j].order })
	full := rendered{}
	appendRendered(&full, leads)
	for _, p := range source {
		appendRendered(&full, p.tree)
	}
	complete := len(full.text) <= capRunes && siblingArticleCovered(*g.article, leads, source)
	if len(full.text) > capRunes {
		full = rendered{}
		appendRendered(&full, leads)
		for _, p := range parts {
			appendRendered(&full, p.tree)
		}
	}
	full = clipSiblingText(full, capRunes)
	for _, p := range parts {
		start, ok := renderedOffset(full, p.tree)
		memberComplete := ok && start+len(p.tree.text) <= len(full.text) && !p.tree.ambiguous
		found := false
		for i := range full.fragments {
			f := &full.fragments[i]
			if f.Identity == p.group.Identity {
				f.Complete = f.Complete && memberComplete
				found = true
			}
		}
		if !found && memberComplete {
			full.fragments = append(full.fragments, Fragment{Identity: p.group.Identity, TextStart: start, TextEnd: start, Role: "body", Complete: true})
		}
	}
	candidates := []Candidate{*g.article}
	for _, member := range g.siblings {
		candidates = append(candidates, member.Best)
	}
	setFragmentCitations(full.fragments, candidates)
	bestMember := g.Identity
	bestStart := min(len(leads.text)+1, len(full.text))
	if len(leads.text) == 0 {
		bestStart = 0
	}
	var best *sourceSpan
	for i := range spans {
		s := &spans[i]
		if s.Role == "body" && (best == nil || s.ContentStart < best.ContentStart) {
			best = s
		}
	}
	if best != nil {
		if node, ok := byNode[best.NodeID]; ok {
			for _, f := range full.fragments {
				if f.Role == "body" && f.Identity == node.Identity && f.Start <= best.Start && best.Start < f.End {
					bestMember = node.Identity
					bestStart = f.TextStart + best.Start - f.Start
					break
				}
			}
		}
	}
	var gaps []Gap
	for _, p := range parts {
		if p.tree.ambiguous {
			gaps = append(gaps, Gap{Kind: "ambiguous_tree_text", DocumentKey: g.Identity.DocumentKey, Anchor: g.Identity.Anchor, Message: "Source tree text overlaps ambiguously."})
			break
		}
	}
	return string(full.text), full.fragments, complete, gaps, bestMember, bestStart, nil
}

func clipSiblingText(full rendered, capRunes int) rendered {
	if len(full.text) > capRunes {
		full.text = full.text[:capRunes]
	}
	fragments := make([]Fragment, 0, len(full.fragments))
	for _, f := range full.fragments {
		if f.TextStart >= len(full.text) {
			continue
		}
		if f.TextEnd > len(full.text) {
			f.TextEnd = len(full.text)
			f.End = f.Start + f.TextEnd - f.TextStart
			f.Complete = false
		}
		fragments = append(fragments, f)
	}
	full.fragments = fragments
	return full
}

func renderedOffset(outer, inner rendered) (int, bool) {
	for _, i := range inner.fragments {
		for _, o := range outer.fragments {
			if i.Identity == o.Identity && i.Role == o.Role && o.Start <= i.Start && i.Start < o.End {
				return o.TextStart + i.Start - o.Start - i.TextStart, true
			}
		}
	}
	return 0, false
}

func siblingArticleCovered(article Candidate, leads rendered, parts []siblingPart) bool {
	expected := renderTree(article.Descendants, article.GroupID)
	if expected.ambiguous {
		return false
	}
	covered := make([]bool, len(expected.text))
	mark := func(start, end int) {
		for i := max(0, start); i < min(end, len(covered)); i++ {
			covered[i] = true
		}
	}
	for _, f := range leads.fragments {
		for _, e := range expected.fragments {
			if f.Identity == e.Identity && e.Start <= f.Start && f.End <= e.End {
				start := e.TextStart + f.Start - e.Start
				mark(start, start+f.End-f.Start)
			}
		}
	}
	members := map[int64]bool{}
	for _, p := range parts {
		members[p.group.Best.GroupID] = true
		if p.tree.ambiguous {
			return false
		}
		offset, ok := renderedOffset(expected, p.tree)
		if !ok {
			return false
		}
		mark(offset, offset+len(p.tree.text))
	}
	for _, p := range article.Descendants {
		if p.Kind == "khoan" && !members[p.ID] {
			return false
		}
		for _, f := range expected.fragments {
			if f.Identity == p.Identity {
				offset := f.Start
				for _, line := range strings.Split(string([]rune(p.Text)[f.Start:f.End]), "\n") {
					size := len([]rune(line))
					if readLabelOnly(line, p.Label) {
						start := f.TextStart + offset - f.Start
						mark(start, start+size)
					}
					offset += size + 1
				}
			}
		}
	}
	for i, r := range expected.text {
		if !unicode.IsSpace(r) && !covered[i] {
			return false
		}
	}
	return true
}
